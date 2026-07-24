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

可视化能力已独立封装在 `github.com/swkings/fsm/visualization` 包中，只交换字符串状态、JSON Payload 和通用 Trace，不依赖本库的泛型 FSM 类型。Server 默认不启动，只有显式调用 `Start`、`Serve`，或将 `Handler()` 挂到已有 HTTP Server 时才启用：

```go
server := visualization.NewServer(
    visualization.WithAddress("127.0.0.1:0"),
    visualization.WithTraceLimit(100),
)
if err := server.Start(); err != nil {
    return err
}
defer server.Close(context.Background())
fmt.Println(server.URL())
```

内置 FSM 采用 push 模式：每次状态转移后主动把快照和增量 Trace 提交给 Server。`path` 使用 `/` 分层，Web 端会按目录聚合和折叠：

```go
registration, err := orderFSM.RegisterVisualization(ctx, server,
    visualization.WithRegistrationID("order-1001"),
    visualization.WithDisplayName("Order 1001"),
    visualization.WithPath("production/orders/checkout"),
)
defer registration.Unregister()

// 忽略注册错误时，也可使用链式便捷方法。
vehicleFSM.Register(server, visualization.WithPath("production/vehicles"))

// 也可让 Server 通过统一自注册协议接入内置 FSM。
registration, err = server.Register(ctx, fsm.FSMVisualizable(orderFSM))
```

### 第三方 FSM 接入

第三方实现只需依赖 `visualization.Registrar`。实现 `Registrable` 后，对象可以自行决定注册配置以及使用 pull 或 push 模式：

```go
func (machine *MyFSM) RegisterVisualization(
    ctx context.Context,
    registrar visualization.Registrar,
) (visualization.Registration, error) {
    return registrar.RegisterPull(ctx, machine,
        visualization.WithRegistrationID(machine.ID),
        visualization.WithPath("region-a/services/orders"),
        visualization.WithPullInterval(3*time.Second),
    )
}

registration, err := server.Register(ctx, machine)
```

- **pull 模式**：实现 `PullSource.Descriptor` 和 `PullSource.Collect`，Server 注册时立即采集一次，之后按间隔主动拉取。默认间隔为 5 秒；采集失败会保留上次成功快照并展示错误。
- **push 模式**：调用 `RegisterPush` 获得 `Pusher`，状态变化时调用 `Push(ctx, snapshot, traces...)`。也可向 `POST /api/push/{id}` 提交 `Snapshot` JSON；首次提交会自动创建 push 注册，适合独立进程或第三方语言接入。
- **生命周期**：注册使用传入的 `ctx`；取消 context、调用 `Unregister` 或关闭 Server 都会结束注册并关闭 `Registration.Done()`。
- **Payload**：可直接填写 `Snapshot.Payload`，或使用 `WithPayloadProvider` 统一生成、脱敏并编码 Payload。provider 的编码错误或 panic 会被隔离并显示在页面中。

这种模型与 Prometheus 的使用习惯相似：常驻且可被访问的数据源适合 pull；短生命周期任务、跨进程客户端或事件驱动的数据源适合 push。

Web 页面实时展示：

- 当前状态、上一个状态和事件。
- Payload JSON 快照及编码错误。
- SVG 状态图、当前状态高亮，以及正在执行和最近完成的状态转移。
- 当前状态可接收的 event 及每个 event 对应的目标状态。
- FSM 按注册 `path` 的 `/` 层级形成可聚合折叠的文件夹树，并支持搜索和折叠状态记忆。
- 左侧目录树贯穿页面全高，支持调整宽度、折叠/展开并记忆状态；主内容区以实时状态图作为全屏画布，可拖动图形并双击复位。
- Payload、Trace 和当前可接收事件以透明浮层展示，可独立收起并记忆展开状态；Header 可调节全部浮层透明度，默认透明度为 85%，并同时作用于 Header 和左侧栏背景。
- 状态节点支持鼠标悬浮或键盘聚焦查看当前/上一状态、进入与离开转移数量，以及可接收事件。
- 紧凑 Header 只保留明暗主题切换；状态图可按当前主题和高亮状态导出 PNG。
- 数据来源模式、pull 采集错误和最近更新时间。
- 成功、失败、取消、过期事件的 Trace、耗时和错误。
- 运行时动态注册或注销的 FSM。

HTTP 接口为 `/api/fsms`、`/api/fsms/{id}`、`POST /api/push/{id}`，实时更新使用 `/api/events` SSE。`Handler()` 可以集成到已有 HTTP 服务。独立启动默认只监听本机；Server 本身不提供鉴权，绑定公网地址时应由应用或反向代理增加认证和访问控制。

可运行示例：

```bash
go run ./examples/server
```

## 可视化

- `DotString` 输出经过转义的 Graphviz DOT 文本。
- `VisualizeGraphviz` 直接调用本机 `dot`，不经过系统 Shell；运行环境需要安装 Graphviz。
- `VisualizeForMermaidWithGraphType` 输出 Mermaid 流程图或状态图。
- `VisualizeMermaid` 会安全覆盖目标文件并返回文件错误。
