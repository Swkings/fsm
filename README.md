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
registration, err := server.Register(ctx, client, visualization.RegistrationConfig{
    DisplayName: "Order Worker",
    Path:        "production/orders",
})
defer registration.Unregister()
```

Server 将注册阶段和运行阶段明确分开：

```go
type Client interface {
    // 仅在 Register 时调用一次，返回 FSM 自身标识和完整状态图。
    VisualizationDefinition() Definition

    // 注册完成后仅返回当前状态、Payload、运行信息和 Trace。
    VisualizationSnapshot(context.Context) (RealtimeData, error)
}

type PushClient interface {
    Client
    SubscribeVisualization(PublishFunc) func()
}

type PublishFunc func(RealtimeData, ...TraceSnapshot) error
```

- **注册阶段**：Server 只调用一次 `VisualizationDefinition`，缓存 FSM 自身 ID、名称、描述、完整状态、完整事件和状态转移图；显示名称、目录、采集模式和 pull 间隔由 `RegistrationConfig` 随 `Register` 传入。
- **pull 模式**：Server 注册时读取一次 `RealtimeData`，之后按照 `RegistrationConfig.PullInterval` 拉取实时数据，默认间隔 5 秒。
- **push 模式**：客户端状态变化时通过 `PublishFunc` 直接提交 `RealtimeData` 和可选的增量 Trace；Server 不会再反向读取或同步静态定义。
- Web/API 使用的 `Snapshot` 由 Server 将缓存的 `Definition` 与最新 `RealtimeData` 合成。当前可接收事件也由 Server 根据当前状态和静态转移图计算。
- 取消注册 context、调用 `Unregister` 或关闭 Server 都会停止 pull/订阅，并关闭 `Registration.Done()`。

第三方 pull FSM 只需要实现两个方法：

```go
func (machine *MyFSM) VisualizationDefinition() visualization.Definition {
    return visualization.Definition{
        ID:           machine.ID,
        Name:         "order",
        States:       []string{"created", "paid", "shipped"},
        Events:       []string{"pay", "ship"},
        Transitions: []visualization.TransitionSnapshot{
            {FromState: "created", Event: "pay", ToState: "paid"},
            {FromState: "paid", Event: "ship", ToState: "shipped"},
        },
    }
}

func (machine *MyFSM) VisualizationSnapshot(
    ctx context.Context,
) (visualization.RealtimeData, error) {
    return visualization.RealtimeData{
        CurrentState: machine.State(),
        Payload:      machine.PayloadJSON(),
    }, nil
}

registration, err := server.Register(ctx, machine, visualization.RegistrationConfig{
    DisplayName:  "Order Service",
    Path:         "region-a/services/orders",
    Mode:         visualization.ModePull,
    PullInterval: 3 * time.Second,
})
```

内置 `fsm.FSM` 已直接实现 `visualization.PushClient`。Definition 直接使用 FSM 的 `instanceID`、`Name`、`Describe` 并自动提取完整状态图；FSM 内部不保存任何可视化配置，事件完成后只推送实时数据：

```go
registration, err := server.Register(ctx, orderFSM, visualization.RegistrationConfig{
    DisplayName: "Order 1001",
    Path:        "production/orders/checkout",
})
```

跨进程 push 客户端使用两阶段协议：

1. 首次向 `POST /api/fsms` 提交完整 `Definition`。
2. 后续向 `POST /api/push/{id}` 只提交 `RealtimeData`。

实时 Push 请求如果夹带 `states`、`events` 或 `transitions` 等静态字段会被拒绝，未注册的 ID 也不会被自动创建。

### 服务发现与远程 Pull

Pull 模式可以跨进程工作。远程 FSM 使用 `NewClientHandler` 暴露统一客户端接口：

```go
mux.Handle(
    "/fsm/order/",
    http.StripPrefix("/fsm/order", visualization.NewClientHandler(orderFSM)),
)
```

客户端接口为：

- `GET {endpoint}/definition`：返回一次性 `Definition`。
- `GET {endpoint}/realtime`：返回持续变化的 `RealtimeData`。

可视化 Server 定义了通用服务发现接口，可接入 Consul、Kubernetes、etcd 或其他注册中心：

```go
type ServiceDiscovery interface {
    Discover(context.Context) ([]DiscoveredService, error)
}

discovery, err := server.RegisterDiscovery(
    ctx,
    myDiscovery,
    visualization.DiscoveryConfig{
        ID:              "production",
        PathPrefix:      "production/discovered",
        RefreshInterval: 30 * time.Second,
        PullInterval:    5 * time.Second,
    },
)
```

内置 `HTTPServiceDiscovery` 会定期请求一个 JSON 服务列表：

```json
{
  "services": [
    {
      "key": "order-1",
      "endpoint": "http://order-1:8080/fsm/order",
      "displayName": "Order 1",
      "path": "orders",
      "pullIntervalSeconds": 2
    }
  ]
}
```

Server 会自动完成以下操作：

- 新发现的服务：调用 `/definition` 获取完整状态图并注册为 pull FSM。
- 已注册服务：按照 pull 周期调用 `/realtime`，由 Server 主动更新实时数据。
- 服务地址或配置变化：自动注销旧实例并重新注册。
- 服务从发现结果中消失：自动注销对应 FSM。
- 拉取失败：保留静态状态图和最后一次实时数据，并通过 `collectionError` 展示错误。

Web Header 的服务发现图标可以直接配置 HTTP 发现地址、目录前缀、发现周期和 pull 周期，并查看已发现数量、已注册数量和最近错误。对应 API 为 `GET/POST /api/discoveries`、`POST /api/discoveries/{id}/refresh` 和 `DELETE /api/discoveries/{id}`。这些配置会让 Server 访问指定网络地址，公开部署时必须为配置 API 增加鉴权和网络访问限制。

Web 页面实时展示：

- 当前状态、Payload、Trace、全部状态转移和当前可接收事件。
- FSM 按注册 `path` 的 `/` 层级形成可聚合折叠的文件夹树。
- 左侧目录树支持调整宽度、折叠/展开并记忆状态。
- 状态图作为全屏画布，可拖动、双击复位并导出 PNG。
- 状态节点支持鼠标悬浮或键盘聚焦查看转移信息。
- Payload、Trace 和事件使用可折叠浮层；Header 可调节浮层、Header 和侧栏透明度，默认 85%。
- 明暗主题切换、pull 采集错误以及动态注册/注销。

HTTP 接口为 `GET/POST /api/fsms`、`GET /api/fsms/{id}`、`POST /api/push/{id}`，实时更新使用 `/api/events` SSE。`Handler()` 可以挂载到已有 HTTP 服务。Server 不提供鉴权，绑定公网地址时应由应用或反向代理增加认证和访问控制。

可运行示例：

```bash
go run ./examples/server
```

## 可视化

- `DotString` 输出经过转义的 Graphviz DOT 文本。
- `VisualizeGraphviz` 直接调用本机 `dot`，不经过系统 Shell；运行环境需要安装 Graphviz。
- `VisualizeForMermaidWithGraphType` 输出 Mermaid 流程图或状态图。
- `VisualizeMermaid` 会安全覆盖目标文件并返回文件错误。
