# SagaFlow Agent Contract

本文件是所有自动化 Agent、AI 编程助手和新贡献者进入 SagaFlow 仓库后的首要契约。开始修改前必须先阅读本文件，再按任务类型阅读 [`.agent/README.md`](.agent/README.md) 指向的补充文档。

## 1. 项目定位

SagaFlow 是面向个人漫剧创作者的本地优先工作台。它不是团队版，也不承担多租户、角色权限、组织、配额或协作审计。

必须保持的产品边界：

- 一个 Go 内核同时提供 HTTP API、内嵌 React 工作台、生成任务和媒体任务执行。
- 一个数据目录承载 SQLite、项目文件、素材、凭证主密钥、日志、备份和按需安装的工具。
- 只支持一个本地账号；默认监听 `127.0.0.1:18848`。
- 本地项目文件是素材主副本；S3/COS/OSS/MinIO 只保存用户显式发布的远端副本。
- 不引入 PostgreSQL、Redis、独立 Worker 或团队版兼容层，除非用户明确改变产品方向。
- FFmpeg 不随正式包捆绑，由工作台或启动器按固定版本下载，也允许使用系统安装。
- Fyne 启动器是内核管理壳，不复制浏览器工作台的业务功能。

更完整的产品和架构说明见 [`.agent/project.md`](.agent/project.md) 与 [`.agent/architecture.md`](.agent/architecture.md)。

## 2. 开始任务前

1. 确认工作目录是 Git 根目录，而不是它的上级目录。
2. 执行 `git status --short` 和 `git branch --show-current`。
3. 保留已有用户改动；不得把无关改动混进当前提交。
4. 阅读与任务直接相关的实现、测试和文档，不凭文件名猜测行为。
5. 涉及厂商模型、API、依赖、许可证或平台能力时，使用当前官方文档核对，不能仅依赖记忆。
6. 先确定验证范围；付费真实生成、写入外部存储或推送远端前必须确认用户授权范围。

## 3. 目录与所有权

| 路径 | 职责 | 主要约束 |
| --- | --- | --- |
| `cmd/sagaflow` | 内核 CLI 入口 | 保持无 GUI 依赖，可用于桌面、服务器和 Docker |
| `cmd/sagaflow-desktop` | 独立 Fyne 启动器模块 | 独立 `go.mod`；不得让 Fyne/CGo 进入内核依赖图 |
| `internal/app` | 运行时组装和进程生命周期 | 依赖装配集中于此，不把业务规则塞进入口 |
| `internal/api` | Fiber 路由、鉴权、请求/响应边界 | 保持薄层；业务逻辑进入 `service` |
| `internal/service` | 业务用例、生成编排、凭证与音色 | 不依赖前端；错误应可映射为稳定 API 响应 |
| `internal/store/db` | SQLite 查询与领域记录 | SQL/事务边界清晰；避免在 handler 中直接写 SQL |
| `internal/platform/sqlite/migrations` | 数据库迁移 | 已发布后只能添加前向迁移，不重写历史迁移 |
| `internal/inference` | 统一模型请求契约和网关 | 不依赖 Fiber、React 或具体数据库结构 |
| `internal/inference/adapters/*` | 厂商协议适配 | 参数必须与模型 Schema 和官方协议同步 |
| `internal/modelcatalog` | 内置模型、Schema 和在线目录包 | 型号、生命周期、默认参数和适配器必须一致 |
| `internal/promptcatalog` | 内置 Prompt | 保持少而轻量；更新时同步目录测试 |
| `internal/platform/storage` | 本地对象与 S3 副本 | 本地为主；远端上传必须显式发生 |
| `internal/queue` | SQLite 持久任务 Worker | 不退化为只保存在内存中的状态 |
| `internal/media`、`internal/media/ffmpeg` | 媒体与 FFmpeg 管理 | 不把 FFmpeg 二进制提交进仓库 |
| `internal/webui/dist` | Go embed 的生产前端 | 由 `web` 构建生成，但必须提交并与源码一致 |
| `web/src/api` | 前端 API 客户端、类型和查询键 | API 变更时同步后端、类型与缓存失效策略 |
| `web/src/features` | 按业务域组织的页面 | 优先拆分领域组件，避免继续扩大巨型页面文件 |
| `.github/workflows/scripts` | CI/Release 构建脚本 | 本地与 Actions 共用发布逻辑，避免把大段脚本内联进 YAML |

## 4. 不可破坏的实现约束

### 数据与安全

- 不提交 `data/`、`bin/`、`dist/`、日志、备份、数据库、凭证、API Key、Cookie 或本地配置。
- 凭证只通过 `CredentialService` 和本机主密钥加密保存，不写入普通配置或日志。
- 备份包含主密钥，必须按敏感文件处理。
- 删除逻辑资产时，不得贸然删除仍可能被其他记录引用的物理文件。
- 监听非回环地址前必须已有账号；不要用“演示方便”为理由永久放宽默认安全边界。

### 数据库

- 当前项目已经发布，修改表结构时新增编号迁移和相应测试。
- 不修改已发布的 `001_initial.sql` 来伪造干净历史。
- 新迁移必须可在已有数据库上前向执行；删除数据或不可逆变换需要显式说明。
- 数据访问放入 `internal/store/db`，业务约束放入 `internal/service` 或数据库约束，避免双重且不一致的规则。

### 模型与生成

- 目录里可选择的模型必须存在可执行适配器；不允许只增加显示项。
- 模型参数 Schema、默认值、前端编辑器、适配器载荷和测试必须对齐。
- 预览、邀测、无权限或未验证型号默认关闭，并在元数据中说明支持级别。
- 厂商下线用生命周期标记，不直接删除会被历史任务引用的模型。
- Provider 请求追踪不得包含 Base64 原文、密钥、签名 URL 中的敏感信息或完整凭证。
- 真实付费生成只在用户明确要求时执行；优先用 `httptest` 验证协议与载荷。

### 前端

- React 页面通过 `web/src/api/client.ts` 访问后端，不在组件里散落重复请求实现。
- TanStack Query 的键集中在 `web/src/api/queryKeys.ts`；变更数据后精确失效相关查询。
- 模态框关闭时必须停止音视频、定时器、轮询、下载或画布监听等副作用。
- 所有可点击元素应有一致的可点击反馈；不得用 hover 改变布局尺寸。
- 修改前端后必须运行生产构建，并检查 `internal/webui/dist` 的变更是否与源码一致。

### 桌面与发布

- 桌面端通过运行时契约管理内核；不得依赖固定开发路径。
- 用户发布包只暴露桌面入口，内核放平台约定的内部目录；独立内核包仍单独发布。
- Release 支持 Windows、Linux、macOS 的 amd64/arm64，Docker 支持 linux/amd64 和 linux/arm64。
- 不手工拼装正式 Release；以 `.github/workflows/release.yml` 和其中的 PowerShell 脚本为准。

## 5. 常用改动入口

- API 或领域功能：`internal/api` → `internal/service` → `internal/store/db` → `web/src/api` → 对应 `web/src/features`。
- 新模型或参数调整：`internal/modelcatalog` → 对应 adapter → adapter/catalog 测试 → 前端参数编辑器兼容性。
- 新 Provider：`internal/providercatalog` → `internal/inference/adapters` → `internal/app/app.go` 注册 → 服务连接与凭证 UI。
- Prompt：`internal/promptcatalog` 与目录同步测试。
- 媒体工具：`internal/service/media_tools.go`、`internal/queue/media.go`、`internal/media`、工具页。
- 桌面启动器：`internal/desktop` 的可测试控制逻辑 + `cmd/sagaflow-desktop` 的 Fyne UI。
- 发布：`.github/workflows/release.yml` 与 `.github/workflows/scripts/`。

详细步骤见 [`.agent/playbooks.md`](.agent/playbooks.md)。

## 6. 验证契约

根据改动范围执行最小充分验证；提交前不得忽略失败：

```powershell
# Go 内核
go test ./...
go vet ./...

# React
Set-Location web
corepack pnpm lint
corepack pnpm test
corepack pnpm build
Set-Location ..

# Fyne 启动器（普通测试；CI 环境使用 -tags ci）
Set-Location cmd/sagaflow-desktop
go test ./...
go vet ./...
Set-Location ../..
```

前端生产构建后必须确认：

```powershell
git diff -- internal/webui/dist
```

提交前的完整门禁、定向测试建议和二进制构建命令见 [`.agent/quality-gates.md`](.agent/quality-gates.md)。

## 7. Git 与任务收尾

- 用户要求每次修改后本地提交；一个逻辑改动对应一个清晰提交。
- 提交前检查 `git status`、工作区 diff、暂存区 diff和敏感信息。
- 只暂存当前任务文件；生成二进制保留在忽略的 `bin/` 中，不提交。
- 默认不 push、不打 tag、不创建 PR；只有用户明确要求时才执行。
- 不使用 `reset --hard`、`clean -fd`、强推或破坏性历史重写。
- 每次仓库任务结束前重新构建 `bin` 中的内核和桌面启动器，供用户本地查看；如果当前平台无法构建桌面端，必须说明原因。
- 最终回复报告：行为变化、验证结果、二进制位置、提交哈希、是否推送，以及任何未做的真实外部测试。

## 8. 文档维护

当架构、目录职责、构建方式、发布矩阵或关键产品边界发生变化时，必须同步更新本文件及 `.agent/` 中对应文档。不要把短期任务状态、个人凭证、临时服务器地址或很快过期的型号数量写入 Agent 契约。
