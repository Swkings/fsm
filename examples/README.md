# Examples

示例均使用仓库当前版本的 `github.com/swkings/fsm`，可在仓库根目录直接运行。

## 基础状态迁移

演示泛型 Payload、`BeforeHandler`、`Handler`、`AfterHandler` 以及 `TransitionTrace`：

```bash
go run ./examples/basic
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
