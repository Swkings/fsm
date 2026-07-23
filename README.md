# FSM

这是一个支持泛型 Payload、三阶段 Handler、迁移追踪以及 Graphviz/Mermaid 可视化的有限状态机库。

## 可运行示例

```bash
go run ./examples/basic
go run ./examples/async
```

完整说明见 [`examples`](./examples/README.md)。

## 使用模板生成代码

```bash
go run gen/main.go -fsm ./gen/door.fsm -tpl ./gen/template -out ./generated
```

参数说明：

- `fsm`：`.fsm` YAML 描述文件。
- `tpl`：模板目录；省略时使用 `gen/template`。
- `out`：生成代码的父目录；省略时使用当前工作目录。
- `c`：覆盖允许用户编辑的 `payload.go` 和各迁移 Handler 文件。工厂、元数据和 `common.go` 始终根据描述文件刷新。

描述文件必须提供 `PackageDir`、`Package` 和 `FSMEnginePackage`。其中 `FSMEnginePackage` 是 FSM 模块父级路径，例如当前模块使用：

```yaml
PackageDir: example.com/project
Package: door
FSMEnginePackage: github.com/swkings
```

生成器会校验状态、事件、初始状态、迁移格式、重复迁移及 Go 标识符，并在输入无效时返回明确错误。

## Handler 执行语义

每条迁移依次执行三个可选阶段：

1. `BeforeHandler`：校验权限和业务前置条件。
2. `Handler`：执行核心业务逻辑。
3. `AfterHandler`：保存派生数据或清理临时数据。

任一阶段返回错误都会终止迁移，目标状态不会提交，错误会同时写入 `TransitionTrace`。Payload 内部业务数据不会自动回滚，需要 Handler 自行保证事务或补偿。

同一 FSM 不允许重入处理事件：Handler 内再次调用 `ProcessEvent`，或其他 goroutine 在事件尚未结束时调用，会立即得到 `ErrorEventProcessing`，不会阻塞死锁。Handler 执行期间若通过 `SetCurrentState` 改变状态，原迁移会返回 `ErrorStateChanged`，避免覆盖外部状态修改。

## 异步事件队列

`SubmitEvent` 会把事件放入状态机内部队列。默认事件为高优先级、永不过期；同一优先级按提交顺序执行。调度优先级为：

1. 尚未开始执行的直接 `ProcessEvent` 或 `ProcessEventContext` 调用。
2. 高优先级异步事件。
3. 低优先级异步事件。

优先级只影响尚未开始的事件，已经进入 Handler 的事件不会被抢占。

### Fire-and-forget

不设置 `ctx` 和 `notify` 表示提交方不关心执行结果。FSM 不创建 context 监听和结果通知，`SubmitEvent` 返回的 receipt 为 `nil`：

```go
receipt, err := machine.SubmitEvent(
    "open",
    fsm.WithEventPriority(fsm.EventPriorityLow),
    fsm.WithEventTTL(5*time.Second),
)
// receipt == nil
```

### 取消与状态通知

`WithEventContext` 和 `WithEventNotify` 必须同时设置或同时省略。成对设置后，提交方可以调用 `cancel()` 取消排队中或正在执行的事件，FSM 会通过 `notify` 发送最终状态：

```go
ctx, cancel := context.WithCancel(context.Background())
notify := make(chan fsm.AsyncEventResult, 1)

receipt, err := machine.SubmitEvent(
    "open",
    fsm.WithEventContext(ctx),
    fsm.WithEventNotify(notify),
)
if err != nil {
    return err
}

cancel() // 可选：向 FSM 发送取消信号
result := <-notify
_ = receipt // 成对设置时非 nil，也可以通过 receipt.Wait 等待
_ = result
```

通知状态包括 `succeeded`、`failed`、`canceled` 和 `expired`。通知通道由调用方持有，FSM 不会关闭它；建议使用容量至少为 1 的缓冲通道，并在收到通知前保持通道可用。

`WithEventTTL` 和 `WithEventExpiresAt` 独立控制事件的排队有效期。过期时若事件仍在队列中，会被立即移出；配置了 `ctx + notify` 时还会收到 `expired` 通知。事件一旦开始执行，过期时间不再取消 Handler。

运行中事件的取消是协作式的，Handler 可通过 `machine.GetEventContext()` 监听提交方的 `ctx.Done()`；若 Handler 长时间阻塞且不监听 context，FSM 无法强制终止它。应用退出时应从 Handler 外部调用 `Close`，用于清理剩余事件并等待队列 worker 结束。

## 可视化

- `DotString` 输出经过转义的 Graphviz DOT 文本。
- `VisualizeGraphviz` 直接调用本机 `dot`，不经过系统 Shell；运行环境需要安装 Graphviz。
- `VisualizeForMermaidWithGraphType` 输出 Mermaid 流程图或状态图。
- `VisualizeMermaid` 会安全覆盖目标文件并返回文件错误。
