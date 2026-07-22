# SagaFlow

SagaFlow 个人版是一套本地优先的 AI 漫剧生产工作台。项目、剧本、素材、生成任务和画布都保存在自己的设备上；Ollama 与 ComfyUI 可以直接读取本地素材，云端模型只在需要参考文件时使用用户手动发布的 S3 副本。

## 设计边界

- 单个全局账号，不包含用户、角色、工作组或资源授权系统。
- Go 单进程同时提供 API、SQLite 持久任务队列和内嵌的 React 工作台。
- SQLite 是唯一数据库，不需要 PostgreSQL、Redis 或独立 worker。
- 本地文件是唯一主副本；S3 是可选的手动发布目标，不会自动同步或替代本地文件。
- 模型服务、模型目录、凭证、ComfyUI 工作流、全局 Prompt 预设、全局音色和 S3 连接都在工作台内维护，不写入运行配置。
- 个人版使用独立的干净数据结构，不兼容团队版数据库，也不会尝试迁移团队版数据。

## 技术栈

- Go 1.26、Fiber 3.4.0、easyhash 1.2.0
- SQLite（WAL）、持久化进程内任务队列
- React 19、TypeScript、Vite、Ant Design、React Flow
- AWS SDK for Go v2，用于 AWS S3、腾讯云 COS、阿里云 OSS、MinIO 等 S3 兼容服务的手动发布

## 本机启动

首次运行先创建唯一账号：

```bash
go run ./cmd/sagaflow account init \
  --username admin \
  --display-name Creator \
  --password "change-this-password"
go run ./cmd/sagaflow serve
```

打开 `http://127.0.0.1:8080`。正式二进制默认使用与自身同目录的 `data/`，便于整套移动和备份；`go run` 开发时使用当前仓库下的 `data/`。也可以仅为当前命令覆盖：

```bash
SAGAFLOW_DATA_DIR=./data go run ./cmd/sagaflow account init --password "change-this-password"
SAGAFLOW_DATA_DIR=./data go run ./cmd/sagaflow serve
```

PowerShell 使用 `$env:SAGAFLOW_DATA_DIR = '.\data'`。如果仅监听回环地址，也可以直接启动并在首次打开页面时创建账号；绑定公网或局域网地址前必须先初始化账号。

运行配置是可选的：

```bash
go run ./cmd/sagaflow config init --output ./data/config.yaml
go run ./cmd/sagaflow serve --config ./data/config.yaml
```

配置只包含数据目录、监听地址、会话和任务执行参数，示例见 [`configs/config.example.yaml`](configs/config.example.yaml)。模型密钥或 S3 密钥不会从该文件或环境变量读取。

## 前端开发

后端运行在 `127.0.0.1:8080` 时：

```bash
cd web
corepack pnpm install --frozen-lockfile
corepack pnpm dev
```

生产构建会写入 `internal/webui/dist`，随后由 Go `embed` 放入同一个二进制：

```bash
make build
```

## Docker

镜像只有一个应用容器和一个数据卷。由于容器监听 `0.0.0.0`，需要先创建账号再启动服务：

```bash
docker compose build
docker compose run --rm sagaflow account init \
  --username admin \
  --display-name Creator \
  --password "change-this-password"
docker compose up -d
```

访问 `http://localhost:8080`。停止服务使用 `docker compose down`；不要加 `-v`，除非确认要删除全部数据。

## 模型和存储

登录后在“模型”页完成以下管理：

- Ollama（`http://127.0.0.1:11434`）和 ComfyUI（`http://127.0.0.1:8188`）连接已预置，启动本地服务后可直接检测、同步模型或导入工作流；
- 添加或编辑 DeepSeek、Seedream、Seedance、MiniMax 等云端服务连接；
- 保存本机加密的服务凭证；
- 同步 Ollama 模型、导入 ComfyUI API 工作流、维护模型参数、全局 Prompt 预设和全局音色。

所有素材会先写入本地内容寻址对象目录。Ollama 与 ComfyUI 使用本地内容流，不需要公网 URL。云端模型需要参考素材时，先在“设置 → S3 发布”添加兼容连接，再从资产操作中手动发布所需文件；生成任务保存的是该资产对应的具体远端副本，不会后台自动上传。

## 运维命令

```bash
sagaflow doctor
sagaflow account reset-password --password "new-password"
sagaflow backup create
sagaflow backup restore --archive /path/to/backup.zip --yes
```

备份包含 SQLite 快照、本地素材和解密凭证所需的主密钥，不包含与机器路径相关的运行配置，应像密码一样保护。恢复前必须停止 SagaFlow；恢复成功后，原数据目录会以 `.before-restore-时间` 后缀保留，确认无误后再手动删除。

Linux systemd、Docker、Windows/macOS 命令行部署详见 [`docs/deployment.md`](docs/deployment.md)，架构和数据边界详见 [`docs/architecture.md`](docs/architecture.md)。

## 验证

```bash
go test ./...
cd web && corepack pnpm lint && corepack pnpm build
```

## License

[MIT](LICENSE)
