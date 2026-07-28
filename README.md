# FSM

这是一个支持泛型 Payload、三阶段 Handler、转移追踪以及 Graphviz/Mermaid 可视化的有限状态机库。

## 可运行示例

```bash
go run ./examples/basic
go run ./examples/generated
go run ./examples/async
go run ./examples/server
```

`examples/generated` 会读取 `examples/generated/config.yaml` 启动可视化 Client Server，并每隔随机 3～5 秒触发当前状态下的一个合法事件。完整说明见 [`examples`](./examples/README.md)。

## 使用模板生成代码

```bash
go run gen/main.go -fsm ./gen/door.fsm -tpl ./gen/template -out ./generated
```

参数说明：

- `fsm`：`.fsm` YAML 描述文件。
- `tpl`：模板目录；省略时使用 `gen/template`。
- `out`：生成代码的父目录；省略时使用当前工作目录。
- `c`：覆盖允许用户编辑的 `payload.go` 和各转移 Handler 文件。工厂、元数据和 `common.go` 始终根据描述文件刷新。

每份描述文件只生成一个状态机，必须使用单个 `FSM` 对象；不再支持 `FSMs` 列表或一份 YAML 生成多个 FSM。描述文件还必须提供 `PackageDir`、`Package` 和 `FSMEnginePackage`，其中 `FSMEnginePackage` 是 FSM 模块父级路径，例如：

```yaml
PackageDir: example.com/project
Package: door
FSMEnginePackage: github.com/swkings
State:
  - Open
  - Close
Event:
  - Lock
  - Unlock
FSM:
  Name: Door
  Initial: Close
  Transition:
    - Close -> Unlock -> Open
    - Open -> Lock -> Close
```

生成器会校验状态、事件、初始状态、转移格式、重复转移及 Go 标识符，并在输入无效时返回明确错误。状态和事件通过 `types.StateEnum.STATE`、`types.EventEnum.EVENT` 枚举访问。每条转移生成独立 Handler 文件，文件名为 `s_STATE_e_EVENT.go`，三阶段方法名分别为 `_Before_s_STATE_e_EVENT`、`_In_s_STATE_e_EVENT`、`_After_s_STATE_e_EVENT`。Handler 直接绑定到当前 FSM，不再生成跨 FSM 的名称判断或共享分发层。

## Handler 执行语义

每条转移依次执行三个可选阶段：

1. `BeforeHandler`：校验权限和业务前置条件。
2. `Handler`：执行核心业务逻辑。
3. `AfterHandler`：保存派生数据或清理临时数据。

任一阶段返回错误都会终止转移，目标状态不会提交，错误会同时写入 `TransitionTrace`。Payload 内部业务数据不会自动回滚，需要 Handler 自行保证事务或补偿。

事件参数可通过 `ProcessEvent(event, params...)` 或 `ProcessEventContext(ctx, event, params...)` 传入。`BeforeHandler`、`Handler` 和 `AfterHandler` 统一使用 `Actor[T]` 签名：`func(*FSM[T], ...any) error`。不使用参数时可将其命名为 `_ ...any`：

```go
handler := fsm.Handler[Payload]{
    Handler: func(machine *fsm.FSM[Payload], params ...any) error {
        attack := params[0].(AttackParams)
        machine.Payload.TargetHealth -= attack.Damage
        return nil
    },
}
machine.SetTransitions(
    fsm.NewTransitionItem(EventAttack, StateIdle, StateAttacking, handler),
)
_, err := machine.ProcessEvent(EventAttack, AttackParams{Damage: 25})
```

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

## 实时可视化

可视化能力位于独立的 `github.com/swkings/fsm/visualization` 包。业务进程和独立聚合服务使用同一个 `Server` 实现，但承担不同职责：

- 业务进程内只有一个全局 Client Server，多个 FSM 在任意模块注册到该 Server，并在状态变化后通过进程内订阅主动 Push。
- Client Server 同时托管当前进程的 React 可视化前端，可直接通过配置的地址访问。
- 独立可视化服务不接收业务代码直接注册；它通过服务发现获得多个 Client Server 地址，再展开并 Pull 每个 Client Server 中的 FSM。

### 配置业务进程 Client Server

`NewServer(config)` 创建或返回进程内全局单例 Server；Server 仍由调用方显式启动：

```go
clientServer := visualization.NewServer(
    visualization.VisualizationConfig{
        ServerAddress: "0.0.0.0:5005",
        TraceLimit:    100,
        PathPrefix:    "production",
    },
)
if err := clientServer.Start(); err != nil {
    return err
}
defer visualization.ShutdownClientServer(context.Background())

fmt.Println(visualization.ClientServerURL())
```

内置 `fsm.FSM` 从 Payload 获取每个实例的显示名称和相对目录：

```go
func (payload OrderPayload) DisplayName() string {
    return payload.OrderName
}

func (payload OrderPayload) Category() string {
    return "orders/checkout"
}

orderRegistration, err := visualization.RegisterClient(ctx, orderFSM)
paymentRegistration, err := visualization.RegisterClient(ctx, paymentFSM)
```

`PathPrefix` 与 Payload 的 `Category()` 使用 `/` 组合，例如上例最终目录为 `production/orders/checkout`；两者都允许为空。Payload 对应方法返回空字符串时，显示名称回退到 FSM 名称，目录仅使用非空部分。

内置 `fsm.FSM` 已实现 `visualization.PushClient`。注册时 Server 只读取一次完整 `Definition`；注册完成后，FSM 事件结束会将 `RealtimeData` 和增量 Trace 主动推送到 Client Server。取消注册 context、调用 `Unregister` 或关闭 Client Server 都会移除订阅。

第三方状态机可以实现相同接口。实现 `PushClient` 时默认采用 Push；只实现 `Client` 时默认采用 Pull。需要指定显示信息、目录、模式或 Pull 周期时，由 Client 实现 `RegistrationConfigProvider`：

```go
type Client interface {
    VisualizationDefinition() Definition
    VisualizationSnapshot(context.Context) (RealtimeData, error)
}

type PushClient interface {
    Client
    SubscribeVisualization(PublishFunc) func()
}

type RegistrationConfigProvider interface {
    VisualizationRegistrationConfig() RegistrationConfig
}
```

### 独立可视化服务聚合多个客户端

注册中心、Consul、Kubernetes 或 etcd 发现的是“业务进程的 Client Server 地址”，不是单个 FSM 地址。独立服务使用 `RegisterClientServerDiscovery` 注册发现源：

```go
aggregate := visualization.NewServerWithConfig(visualization.ServerConfig{
    Address:    "0.0.0.0:5005",
    TraceLimit: 100,
})
if err := aggregate.Start(); err != nil {
    return err
}

discovery, err := aggregate.RegisterClientServerDiscovery(
    ctx,
    &visualization.HTTPServiceDiscovery{
        Endpoint: "http://registry:8080/visualization-servers",
    },
    visualization.DiscoveryConfig{
        ID:              "production",
        PathPrefix:      "production",
        RefreshInterval: 30 * time.Second,
        PullInterval:    5 * time.Second,
    },
)
```

`HTTPServiceDiscovery.Endpoint` 既可以填写返回 Client Server 列表的注册中心接口，也可以直接填写单个 Client Server 基础地址，例如 `http://order-service-1:5005`。直接地址会通过 `/api/targets` 自动识别并作为单实例发现源添加。

HTTP 注册中心接口返回 Client Server 列表：

```json
{
  "services": [
    {
      "key": "order-service-1",
      "endpoint": "http://order-service-1:5005",
      "path": "orders"
    },
    {
      "key": "payment-service-1",
      "endpoint": "http://payment-service-1:5005",
      "path": "payments"
    }
  ]
}
```

聚合服务会执行以下流程：

- 请求每个 Client Server 的 `GET /api/targets`，获取该进程当前注册的全部 FSM。
- 对每个 FSM 分别请求 `/api/clients/{id}/definition` 和 `/api/clients/{id}/realtime`。
- 以 Client Server 的发现 key 为 FSM ID 增加来源命名空间，因此不同进程可以注册相同的本地 FSM ID。
- Client Server 新增或注销 FSM、服务地址发生变化或实例从发现结果中消失时，聚合服务会自动同步。
- Pull 失败时保留静态状态图与最后一次实时数据，并通过 `collectionError` 展示错误。

Web Header 最右侧的设置图标可进入设置页，并在“服务发现”设置栏中配置 HTTP 注册中心接口或直接 Client Server 地址、目录前缀、发现周期和 Pull 周期。对应 API 为 `GET/POST /api/discoveries`、`POST /api/discoveries/{id}/refresh` 和 `DELETE /api/discoveries/{id}`。这些配置会让 Server 访问指定网络地址，公开部署时必须增加鉴权和网络访问限制。

底层 `RegisterDiscovery` 与 `NewClientHandler` 仍可用于直接接入单个、非 Client Server 管理的第三方远程状态机；业务进程和独立聚合服务应优先采用上述 Client Server 架构。

Web 页面实时展示：

- 当前状态、Payload、Trace、全部状态转移和当前可接收事件。
- FSM 按注册 `path` 的 `/` 层级形成可聚合折叠的文件夹树。
- 左侧目录树支持调整宽度、折叠/展开并记忆状态。
- 状态图作为全屏画布，右侧竖向工具栏提供放大、缩小、恢复默认视图和 PNG 导出；画布也支持连续拖动。
- 状态节点支持鼠标悬浮或键盘聚焦查看转移信息。
- Active Event 与 Payload 由顶部 Nav 独立打开或关闭；Trace 固定在底部状态栏并可展开完整记录。Header 可调节浮层和侧栏透明度，默认 85%。
- 明暗主题切换、pull 采集错误以及动态注册/注销。

HTTP 接口包括 `GET/POST /api/fsms`、`GET /api/fsms/{id}`、`POST /api/push/{id}`；Client Server 另提供 `GET /api/targets`、`GET /api/clients/{id}/definition` 和 `GET /api/clients/{id}/realtime` 供独立服务聚合，实时更新使用 `/api/events` SSE。`Handler()` 可以挂载到已有 HTTP 服务。Server 不提供鉴权，绑定公网地址时应由应用或反向代理增加认证和访问控制。

可视化采用前后端分离结构：React/Vite 源码位于 `visualization/frontend`，Go 包只提供 `/api/*` 与 SSE，并通过 `go:embed` 托管生产构建。开发时分别启动 Go Server 与 `npm run dev`，Vite 默认把 `/api` 代理到 `http://127.0.0.1:5005`；生产构建可执行 `go generate ./visualization` 或在前端目录执行 `npm ci && npm run build`。

可运行示例：

```bash
go run ./examples/server
```

## 可视化

- `DotString` 输出经过转义的 Graphviz DOT 文本。
- `VisualizeGraphviz` 直接调用本机 `dot`，不经过系统 Shell；运行环境需要安装 Graphviz。
- `VisualizeForMermaidWithGraphType` 输出 Mermaid 流程图或状态图。
- `VisualizeMermaid` 会安全覆盖目标文件并返回文件错误。
