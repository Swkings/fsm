# FSM

这是一个支持泛型 Payload、三阶段 Handler、转移追踪以及 Graphviz/Mermaid 可视化的有限状态机库。

## 可运行示例

```bash
go run ./examples/basic
go run ./examples/async
go run ./examples/server
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
- `c`：覆盖允许用户编辑的 `payload.go` 和各转移 Handler 文件。工厂、元数据和 `common.go` 始终根据描述文件刷新。

描述文件必须提供 `PackageDir`、`Package` 和 `FSMEnginePackage`。其中 `FSMEnginePackage` 是 FSM 模块父级路径，例如当前模块使用：

```yaml
PackageDir: example.com/project
Package: door
FSMEnginePackage: github.com/swkings
```

生成器会校验状态、事件、初始状态、转移格式、重复转移及 Go 标识符，并在输入无效时返回明确错误。

## Handler 执行语义

每条转移依次执行三个可选阶段：

1. `BeforeHandler`：校验权限和业务前置条件。
2. `Handler`：执行核心业务逻辑。
3. `AfterHandler`：保存派生数据或清理临时数据。

任一阶段返回错误都会终止转移，目标状态不会提交，错误会同时写入 `TransitionTrace`。Payload 内部业务数据不会自动回滚，需要 Handler 自行保证事务或补偿。

同一 FSM 不允许重入处理事件：Handler 内再次调用 `ProcessEvent`，或其他 goroutine 在事件尚未结束时调用，会立即得到 `ErrorEventProcessing`，不会阻塞死锁。Handler 执行期间若通过 `SetCurrentState` 改变状态，原转移会返回 `ErrorStateChanged`，避免覆盖外部状态修改。

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

## 实时可视化 Server

可视化能力位于独立的 `github.com/swkings/fsm/visualization` 包。Server 使用一个普通配置结构创建，不再使用多组 Option：

```go
server := visualization.NewServerWithConfig(visualization.ServerConfig{
    Address:    "127.0.0.1:0",
    TraceLimit: 100,
})
if err := server.Start(); err != nil {
    return err
}
defer server.Close(context.Background())
```

### 注册接口

进程内客户端只有一个注册入口：

```go
registration, err := server.Register(ctx, client)
defer registration.Unregister()
```

Server 定义两个直接接口：

```go
type Client interface {
    VisualizationConfig() ClientConfig
    VisualizationSnapshot(context.Context) (Snapshot, error)
}

type PushClient interface {
    Client
    SubscribeVisualization(PublishFunc) func()
}
```

- **pull 客户端**只实现 `Client`。Server 注册时立即读取一次快照，之后按照 `ClientConfig.PullInterval` 主动拉取，默认间隔 5 秒。
- **push 客户端**额外实现 `PushClient`。Server 会传入一个 `PublishFunc`；客户端状态变化时调用它，Server 随后自行读取最新快照。客户端只需返回取消订阅函数。
- 注册元数据全部放在一个 `ClientConfig` 中，包括 ID、名称、path、模式和 pull 间隔，不再经过注册器回调或 Option 链。
- 取消注册 context、调用 `Unregister` 或关闭 Server 都会停止 pull/订阅，并关闭 `Registration.Done()`。

第三方 pull FSM 的完整实现只需要两个方法：

```go
func (machine *MyFSM) VisualizationConfig() visualization.ClientConfig {
    return visualization.ClientConfig{
        ID:           machine.ID,
        Name:         "order",
        Path:         "region-a/services/orders",
        Mode:         visualization.ModePull,
        PullInterval: 3 * time.Second,
    }
}

func (machine *MyFSM) VisualizationSnapshot(
    ctx context.Context,
) (visualization.Snapshot, error) {
    return visualization.Snapshot{
        CurrentState: machine.State(),
        Transitions:  machine.Transitions(),
        Payload:      machine.PayloadJSON(),
    }, nil
}

registration, err := server.Register(ctx, machine)
```

内置 `fsm.FSM` 已直接实现 `visualization.PushClient`，只需按需设置展示元数据后注册：

```go
orderFSM.SetVisualizationConfig(visualization.ClientConfig{
    ID:          "order-1001",
    DisplayName: "Order 1001",
    Path:        "production/orders/checkout",
})
registration, err := server.Register(ctx, orderFSM)
```

跨进程客户端仍可向 `POST /api/push/{id}` 提交 `Snapshot` JSON；首次提交会自动创建 push 注册。

Web 页面实时展示：

- 当前状态、Payload、Trace、全部状态转移和当前可接收事件。
- FSM 按注册 `path` 的 `/` 层级形成可聚合折叠的文件夹树。
- 左侧目录树支持调整宽度、折叠/展开并记忆状态。
- 状态图作为全屏画布，可拖动、双击复位并导出 PNG。
- 状态节点支持鼠标悬浮或键盘聚焦查看转移信息。
- Payload、Trace 和事件使用可折叠浮层；Header 可调节浮层、Header 和侧栏透明度，默认 85%。
- 明暗主题切换、pull 采集错误以及动态注册/注销。

HTTP 接口为 `/api/fsms`、`/api/fsms/{id}`、`POST /api/push/{id}`，实时更新使用 `/api/events` SSE。`Handler()` 可以挂载到已有 HTTP 服务。Server 不提供鉴权，绑定公网地址时应由应用或反向代理增加认证和访问控制。

可运行示例：

```bash
go run ./examples/server
```

## 可视化

- `DotString` 输出经过转义的 Graphviz DOT 文本。
- `VisualizeGraphviz` 直接调用本机 `dot`，不经过系统 Shell；运行环境需要安装 Graphviz。
- `VisualizeForMermaidWithGraphType` 输出 Mermaid 流程图或状态图。
- `VisualizeMermaid` 会安全覆盖目标文件并返回文件错误。
