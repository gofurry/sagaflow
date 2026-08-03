# Quality Gates

## 最小验证矩阵

| 改动 | 必须执行 |
| --- | --- |
| Go 业务/API/存储 | 定向 `go test`、`go test ./...`、`go vet ./...` |
| 数据库迁移 | 空库迁移测试、已有库升级测试、相关 store/service 测试 |
| Provider/模型 | catalog 测试、adapter 载荷测试、`go test ./...` |
| React/样式 | `pnpm lint`、`pnpm test`、`pnpm build`、检查嵌入 dist |
| 桌面逻辑 | `go test ./...`、`go vet ./...`（desktop module）；需要时 `-tags ci` |
| CI/打包 | 脚本参数校验、相关本机构建；无法覆盖的平台交给 Actions 矩阵 |
| 纯文档 | 链接/路径检查、`git diff --check`；仍按项目约定刷新本地 bin |

## 完整本地回归（PowerShell）

```powershell
go test ./...
go vet ./...

Push-Location web
corepack pnpm lint
corepack pnpm test
corepack pnpm build
Pop-Location

Push-Location cmd/sagaflow-desktop
go test ./...
go vet ./...
Pop-Location

git diff --check
```

如果本机缺少 Fyne 图形依赖，可执行：

```powershell
Push-Location cmd/sagaflow-desktop
go test -tags ci ./...
go vet -tags ci ./...
Pop-Location
```

## 开发产物

仓库约定任务结束时在 `bin/` 留下可查看的 Windows 开发产物。版本值应使用当前目标版本或用户指定值，不要从旧 README 示例盲目复制。

```powershell
$version = (git describe --tags --always --dirty)
go build -trimpath -ldflags "-s -w -X main.version=$version" -o .\bin\sagaflow.exe .\cmd\sagaflow

Push-Location cmd/sagaflow-desktop
go build -trimpath -ldflags "-s -w -X main.version=$version" -o ..\..\bin\sagaflow-desktop.exe .
Pop-Location
```

`bin/` 被 Git 忽略。不要使用 `git add -f` 提交二进制。

正式跨平台包使用：

- `.github/workflows/scripts/build.ps1`
- `.github/workflows/scripts/build-desktop.ps1`
- `.github/workflows/scripts/archive-package.ps1`

## 真实生产验证

真实外部测试不是普通单元测试的替代品。只有用户要求时才执行，并遵守：

- 使用已配置的本地开发凭证，不把值打印到终端或回复。
- 采用最小生成数量、短时长和低成本分辨率。
- 明确哪些步骤使用真实 API，哪些是 mock/本地验证。
- 等待异步任务完成或明确取消；任务结束后关闭本次启动的进程。
- 检查本地入库、采用、S3 发布、画布引用、预览、下载和媒体合片的完整闭环。
- 不删除用户已有项目或凭证；需要清理时只处理本次测试创建且已确认的对象。

## 提交前检查

```powershell
git status --short
git diff
git diff --check
git diff --staged
```

逐项确认：

- 暂存区只包含当前任务。
- `internal/webui/dist` 与前端源码一致。
- 没有 `.env`、数据库、日志、备份、二进制或密钥。
- 没有意外依赖更新；依赖文件变更成对出现。
- 新行为有测试，失败路径和取消路径得到考虑。
- 用户可见行为与 README/docs/Agent 文档保持一致。

## 交付格式

最终回复至少包含：

- 完成了什么以及用户可见结果；
- 通过的测试和未执行的测试；
- `bin` 产物位置；
- 本地提交哈希和提交信息；
- 是否 push；
- 存在的限制、邀测能力或外部依赖。

安全回滚优先使用：

```powershell
git revert <commit>
```
