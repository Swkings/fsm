# Examples

示例均使用仓库当前版本的 `github.com/swkings/fsm`，可在仓库根目录直接运行。

## 基础状态转移

演示泛型 Payload、`BeforeHandler`、`Handler`、`AfterHandler` 以及 `TransitionTrace`：

```bash
go run ./examples/basic
```

## 实时可视化 Server

演示两种接入方式：本库 FSM 在状态变化后主动 push；一个不依赖 `fsm.FSM` 的第三方状态机实现 `visualization.Registrable` 和 `PullSource`，由 Server 定时 pull。两者通过 `path` 分入多级文件夹。

Web 页面以可拖动状态图作为全屏画布，Payload、Trace 和当前可接收事件使用可折叠透明浮层；同时支持文件夹聚合搜索、节点悬浮信息、侧栏折叠、浮层透明度调节、明暗主题切换和状态图 PNG 导出：

```bash
go run ./examples/server
```

程序会输出本机访问 URL，按 `Ctrl+C` 退出。

## 异步事件队列

演示以下语义：

- 直接事件、高低优先级异步事件及 TTL。
- `ctx` 与 `notify` 成对设置后的取消和状态通知。
- 不设置 `ctx`、`notify` 时的 fire-and-forget 提交，receipt 为 `nil`。

```bash
go run ./examples/async
```

异步示例会稳定输出 `direct high low` 的执行顺序；取消事件不会进入业务 Handler。
