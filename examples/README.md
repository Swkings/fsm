# Examples

示例均使用仓库当前版本的 `github.com/swkings/fsm`，可在仓库根目录直接运行。

## 基础状态转移

演示泛型 Payload、`BeforeHandler`、`Handler`、`AfterHandler` 以及 `TransitionTrace`：

```bash
go run ./examples/basic
```

## 实时可视化 Server

演示统一的 `server.Register(ctx, client, config)` 接口：注册时一次性提交完整 Definition，后续仅通过 push 或 pull 同步 RealtimeData。本库 FSM 直接实现 `PushClient`，第三方状态机实现 `Client` 并由 Server 定时 pull；两者通过 `path` 分入多级文件夹。

可视化 Server 还支持通过 `RegisterDiscovery` 或 Web Header 中的服务发现配置，自动发现远程客户端、读取 Definition、注册 FSM 并持续 pull RealtimeData。

React Web 页面以可拖动状态图作为全屏画布；Active Event 与 Payload 可由顶部 Nav 独立打开或关闭，Trace 位于底部可展开状态栏。右侧竖向工具栏提供缩放、复位和 PNG 导出，同时支持文件夹聚合搜索、节点悬浮信息、侧栏折叠、透明度调节和明暗主题切换：

```bash
go run ./examples/server
```

程序会输出本机访问 URL，按 `Ctrl+C` 退出。

## 服务发现与远程 Pull

演示远程客户端通过 `NewClientHandler` 暴露 Definition 和 RealtimeData、HTTP 注册中心返回服务列表，以及可视化 Server 自动发现、注册、主动 pull 和动态渲染：

```bash
go run ./examples/discovery
```

## 异步事件队列

演示以下语义：

- 直接事件、高低优先级异步事件及 TTL。
- `ctx` 与 `notify` 成对设置后的取消和状态通知。
- 不设置 `ctx`、`notify` 时的 fire-and-forget 提交，receipt 为 `nil`。

```bash
go run ./examples/async
```

异步示例会稳定输出 `direct high low` 的执行顺序；取消事件不会进入业务 Handler。
