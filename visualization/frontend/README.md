# FSM Observatory Frontend

FSM Observatory 的 React 前端。它只通过 `/api/*` 与 Go Server 通信，实时更新使用 `/api/events` SSE。

## 本地开发

先在仓库根目录启动 Go API Server：

```bash
go run ./examples/server
```

再启动 Vite 开发服务器：

```bash
cd visualization/frontend
npm ci
npm run dev
```

默认访问 `http://127.0.0.1:5173`。Vite 会把 `/api` 代理到 `http://127.0.0.1:5005`；可通过 `FSM_API_TARGET` 修改目标：

```bash
FSM_API_TARGET=http://127.0.0.1:8080 npm run dev
```

## 生产构建

```bash
npm ci
npm run build
```

产物写入 `dist/`，由 `visualization/web.go` 使用 `go:embed` 打进 Go 二进制。修改前端源码后应重新构建并提交 `dist/`，也可以在仓库根目录执行：

```bash
go generate ./visualization
```
