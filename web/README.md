# SagaFlow Web

个人版内嵌工作台，使用 React、TypeScript、Ant Design、React Flow、TanStack Query 和 Vite。

```bash
corepack pnpm install --frozen-lockfile
corepack pnpm dev
corepack pnpm lint
corepack pnpm build
```

开发服务器将 `/api` 与 `/health` 代理到 `http://127.0.0.1:18848`。生产输出直接写入 `../internal/webui/dist`，由 Go `embed` 编译进同一个 SagaFlow 二进制，不单独部署前端容器或 Nginx。
