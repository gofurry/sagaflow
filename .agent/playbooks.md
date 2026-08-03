# Change Playbooks

## 新增或修改 API 功能

1. 在 `internal/api/router.go` 找到资源边界和现有命名。
2. 在 `internal/store/db` 增加最小数据操作；需要表结构时添加新迁移。
3. 在 `internal/service` 实现事务与业务规则。
4. API handler 只做请求解析、调用 service/store 和响应映射。
5. 更新 `web/src/api/types.ts`、`client.ts`、`queryKeys.ts`。
6. 在对应 `web/src/features` 接入并精确刷新缓存。
7. 增加后端定向测试和必要的前端纯逻辑测试。

## 修改数据库结构

1. 确认当前最高迁移编号。
2. 新增 `NNN_description.sql`，包含 Goose Up/Down；不可逆操作要说明。
3. 为旧数据库升级路径添加测试，而不仅测试空库。
4. 更新 `internal/store/db` 模型与查询。
5. 检查备份恢复、删除级联、索引、唯一约束和 JSON 校验。
6. 不清空用户开发数据库，也不复制其中凭证到仓库。

## 新增 Provider

1. 在 `internal/providercatalog` 定义连接能力与官方入口。
2. 在 `internal/inference/adapters/<provider>` 实现 Driver。
3. 请求必须使用统一 `inference.Request`；响应必须返回流式 `Artifact.Content`。
4. 在 `internal/app/app.go` 注册 adapter。
5. 在数据库种子或前向迁移中增加 Provider 记录。
6. 更新模型目录、凭证测试、服务连接 UI 和官方密钥链接。
7. 使用 `httptest` 覆盖鉴权、路径、载荷、异步轮询、错误和敏感追踪。

## 新增或更新模型

1. 只使用厂商当前官方文档确认模型状态和参数。
2. 判断定义放在 `definitions.go` 还是 `model-catalog.json`，遵循该 Provider 现有方式。
3. 更新能力、输入模态、Schema、默认值、支持级别和生命周期。
4. 更新 adapter 参数映射；区分文生、图生、首尾帧、参考生等模式。
5. 支持发现的 Provider 同步更新 discovery 映射。
6. 增加目录数量/存在性测试和载荷契约测试。
7. 邀测或未真实验证型号默认关闭。
8. 如执行真实测试，使用最低成本输入并报告实际调用的模型，不记录密钥。

## 修改内置 Prompt

1. 修改 `internal/promptcatalog` 对应能力文件。
2. 保持 Prompt 模型无关，除非任务本身是厂商专用能力。
3. 内置 Prompt 应少而轻量，不保留已删除 Prompt 的兼容包袱，除非已有发布数据要求迁移。
4. 更新目录同步和精确内容测试。

## 修改前端页面

1. 先确认问题是数据契约、缓存、布局还是副作用生命周期。
2. 复用 API client、查询键、资源选择器和通用预览组件。
3. 大页面新增复杂能力时拆成 feature 内组件或 hook。
4. 模态框销毁/关闭时清理媒体播放、Object URL、timer、AbortController 和查询轮询。
5. 检查空状态、加载状态、错误状态、窄屏和长文本。
6. 运行 lint、test、build，并提交新的 `internal/webui/dist`。

## 修改生成参考或画布关系

1. 同时理解 `canvas_nodes/canvas_edges`、选中视频资产和生成参考快照。
2. 区分“画布可见资源”“已采用资产”“分镜选中成片”和“生成输入引用”。
3. 连接线必须有稳定 source/target handle，保存后可重建。
4. 双击预览和编辑不能让音视频在模态框关闭后继续播放。
5. 增加纯函数测试验证连接到生成引用的转换。

## 修改媒体工具或 FFmpeg 安装

1. 保持系统发现、托管安装和手动安装三条路径。
2. 下载必须有固定版本、架构映射、校验和、进度与取消。
3. 代理配置只作用于本次下载，不持久化明文账户密码。
4. 媒体任务参数应保存快照并可在任务列表诊断。
5. 不提交 FFmpeg 文件或第三方许可证全文到无关文档。

## 修改桌面启动器

1. 优先把逻辑放在 `internal/desktop` 并测试。
2. UI 只负责展示状态、收集输入和调用控制器。
3. 保持窗口关闭、托盘退出、内核停止和外部内核连接的所有权规则。
4. 检查三平台路径、数据目录和发布包内部内核位置。
5. 在本机运行普通测试；CI 使用 `go test -tags ci ./...`。

## 修改 CI 或 Release

1. 优先修改 `.github/workflows/scripts`，YAML 只做矩阵和编排。
2. CI 要验证嵌入前端没有过期。
3. Release 已存在时上传资产必须使用可覆盖逻辑，不能再次无条件创建 Release。
4. 检查 core、desktop、Docker、model catalog、checksums 五类产物。
5. 不在本地提交 `dist/` Release 包。
