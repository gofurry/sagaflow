# Architecture Map

## 进程拓扑

```text
Fyne Launcher（可选）
        │ 启停、健康检查、运行时文件、维护命令
        ▼
SagaFlow Core
├── Fiber HTTP API
├── embedded React UI
├── application services
├── generation gateway + provider adapters
├── SQLite generation/media workers
├── SQLite metadata
├── local project/object storage
└── optional S3 remote exports
```

入口装配集中在 `internal/app/app.go`。任何新服务、Provider driver 或后台 Worker 都应从这里看其生命周期，不要在包级 `init()` 隐式启动长期任务。

## 后端分层

### CLI 与进程

- `cmd/sagaflow/main.go`：版本注入与 CLI 启动。
- `internal/cli`：`serve`、账号、配置、备份、诊断、systemd 等命令。
- `internal/config`：仅处理运行参数，不保存业务凭证。
- `internal/runtimecontract`：内核与桌面端之间的 API/数据版本、运行时文件和控制头约定。

### HTTP 边界

- `internal/api/router.go` 是路由总表，也是快速判断功能入口的第一站。
- 公共路由只有健康检查和认证初始化/登录；业务路由使用单账号会话鉴权。
- API 层负责解析、边界校验、状态码和序列化，不应承载跨资源事务。
- 大文件上传、下载和 Range 请求已有专门限制与测试，新增入口应复用相关保护。

### 业务与持久化

- `internal/service` 编排生成、模型连接、音色、工作流、凭证、S3 和媒体用例。
- `internal/store/db` 提供领域记录和 SQLite 操作。
- `internal/platform/sqlite` 打开数据库并执行 Goose 迁移。
- `internal/queue` 的 Worker 从 SQLite 原子领取任务；重启后运行中任务会进入 `interrupted`，不能假装成功。

推荐调用方向：

```text
api -> service -> store/platform/inference
```

不要反向让 `store`、`inference` 或 `platform` 依赖 Fiber。

## 生成链路

```text
POST /api/generation-jobs
  -> API 校验目标与参考素材
  -> GenerationService 创建快照和 queued job
  -> SQLite queue claim
  -> 解析凭证、连接、模型/工作流与输入
  -> inference.Gateway 路由到 adapter
  -> adapter 发出标准事件与产物
  -> 本地保存为 staged asset
  -> 用户采用后成为正式 asset
```

关键包：

- `internal/inference/contract.go`：统一请求、输入、事件和产物。
- `internal/inference/gateway.go`：Provider 路由。
- `internal/inference/adapters/*`：厂商协议。
- `internal/service/generation.go`：业务编排与落库。
- `internal/store/db/generations.go`、`generation_invocations.go`：任务与可诊断调用记录。
- `web/src/features/generation`：生成页、参考选择和图像编辑工作台。

参考素材有本地对象、临时上传、在线 URL 导入、资产库和 S3 副本等来源。是否需要公网 URL 由 adapter 能力决定，不能在整个生成系统里一刀切要求 S3。

## 模型目录

模型信息有三种来源：

1. `internal/modelcatalog/definitions.go` 的编译期定义；
2. `internal/modelcatalog/model-catalog.json` 的内嵌/Release 目录；
3. Provider 在线发现结果。

目录同步在内核启动时发生。在线目录更新只能覆盖模型描述和生命周期，不能静默覆盖用户的启用选择。未知远端模型只有在存在可靠通用协议和 Schema 时才允许自动导入。

新增或更新模型时同时检查：型号 ID、能力、输入模态、特性、参数 Schema、默认值、支持级别、生命周期、adapter 载荷、发现映射和测试。

## 素材与存储

- `internal/platform/storage` 保存本地字节并维护项目可读布局。
- `staged_assets` 是生成/上传后的候选区；`assets` 是正式资产记录。
- S3 连接和 `asset_remote_exports` 只描述显式发布的副本。
- API 下载必须返回正确文件名、扩展名、MIME 和 Range 行为。
- 文件管理器“打开目录”是桌面本机能力，在无 GUI 的服务器环境应明确失败或降级。

## 媒体链路

媒体任务与生成任务使用独立队列但相同 SQLite 持久化原则：

```text
tools page -> media job -> media queue -> MediaToolsService -> FFmpeg/FFprobe -> staged/asset output
```

FFmpeg 发现和安装位于 `internal/media/ffmpeg`。安装必须支持直接、代理、手动三种路径以及进度、取消和 SHA-256 校验。

## 前端结构

- React 19 + TypeScript + Vite + Ant Design + TanStack Query。
- `web/src/App.tsx` 负责应用壳与路由级组合。
- `web/src/api` 是唯一 HTTP 契约层。
- `web/src/features/*` 按项目、剧本、资产、画布、生成、模型、设置和工具分域。
- `internal/webui/dist` 是 Vite 的生产输出并通过 `go:embed` 进入内核。

后端契约变化时优先修改共享 TS 类型和 client 方法，再修改页面。避免在多个页面各自实现相同轮询、下载或错误处理。

## 桌面结构

- `internal/desktop`：可单测的进程控制、日志、文件打开和资源路径逻辑。
- `cmd/sagaflow-desktop`：Fyne UI 和资源，独立 Go module。
- `packaging`：图标、manifest 和平台打包资源。
- `.github/workflows/scripts/build-desktop.ps1`：正式桌面包布局的唯一权威脚本。

桌面 UI 需要跨平台。调用系统功能前先确认 Windows、Linux、macOS 的实现或合理降级，并使用 `-tags ci` 保持无图形 CI 测试可运行。
