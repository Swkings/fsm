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

`config.Config` 表示 FSM 子系统配置，第三方应用将其作为自身配置的顶层 `FSM` 字段使用；Store 与 Visualization 等能力统一位于该字段下：

```go
type AppConfig struct {
    FSM config.Config
    // 其它业务配置...
}
```

```yaml
FSM:
  Store:
    JSONL:
      Directory: data/fsm
  Visualization:
    ServerAddress: 127.0.0.1:5005
  MCP:
    Enabled: true
    ServerAddress: 127.0.0.1:5006
    Endpoint: /mcp
```

## 统一服务启动与注册

`github.com/swkings/fsm/server` 包中的 `server.Server` 统一管理 FSM 注册，以及按配置启用的 Store、可视化 HTTP Server 和 MCP Server，并直接实现 go-zero 的 `service.Service`。将顶层 FSM 配置注入 Server 后，可以加入应用已有的 `ServiceGroup` 统一启动和停止：

```go
s := server.MustNewServer(appConfig.FSM)

orderFSM := newOrderFSM(&OrderPayload{/* ... */})
paymentFSM := newPaymentFSM(&PaymentPayload{/* ... */})

// 两种写法等价；只会接入配置中启用的能力。
if err := s.Register(orderFSM); err != nil {
    return err
}
if err := paymentFSM.RegisterTo(s); err != nil {
    return err
}

services := service.NewServiceGroup()
services.Add(s)
services.Start()
```

配置 Store 后，默认 key 根据 Payload `Category()`、FSM 名称与 `DisplayName()` 生成。需要明确的稳定业务键进行重启恢复时，使用 `s.RegisterWithKey("orders/order-1001", orderFSM)`；未配置 Store 时调用该方法会返回 `ErrStoreNotConfigured`。Server 停止时会解除所有注册并关闭其持有的 Store；FSM 实例本身仍由业务代码负责 `Close()`。

`NewServer(config)` 返回 `(*Server, error)`；`MustNewServer(config)` 在已配置的子服务初始化失败时直接 panic，适合 go-zero 应用初始化阶段。`config.Config{}` 是合法配置，只管理 FSM 生命周期，不创建 `data/fsm`、不监听可视化或 MCP 端口：

```go
s := server.MustNewServer(config.Config{})
_ = s.Register(machine)
services.Add(s)
```

Store 仅在配置了 `Driver`、JSONL、MySQL 或 Redis 字段时创建；Visualization 仅在 `ServerAddress` 非空时监听；MCP 仅在 `Enabled: true` 或 `ServerAddress` 非空时启动。直接调用 `store.New(config.Store{})` 时仍保留默认 JSONL 行为。

## 包分层与扩展接口

无行为的数据模型集中在 `ds`：根级 `ds` 保存 FSM 与跨模块协议公用模型，`store/ds`、`visualization/ds`、`mcp/ds` 和 `gen/ds` 保存领域模型。日常调用仍可使用 `fsm.FSMState`、`store.Record`、`visualization.Snapshot` 等便捷别名，无需为了常用操作额外导入 `ds`。

仓库提供的接口统一使用 `I` 后缀，包括 `fsm.IPayload`、`store.IStore`、`server.IRegisterableFSM`、`visualization.IClient` 和 `visualization.IPushClient`。旧接口名称不再提供兼容别名；从旧版本迁移时只需更新类型约束与接口声明，实现方法本身不变。

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

事件参数可通过 `ProcessEvent(event, params...)` 或 `ProcessEventWithContext(ctx, event, params...)` 传入。Payload 在创建 FSM 时以指针传入，泛型 `T` 因而是 `*Payload`；调用方和 Handler 始终修改同一个对象。`BeforeHandler`、`Handler` 和 `AfterHandler` 统一使用 `Actor[T]` 签名：`func(*FSM[T], ...any) error`。不使用参数时可将其命名为 `_ ...any`：

```go
payload := &Payload{TargetHealth: 100}
machine := fsm.NewFSM[*Payload]("player", StateIdle, payload)
handler := fsm.Handler[*Payload]{
    Handler: func(machine *fsm.FSM[*Payload], params ...any) error {
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

`Metadata()` 返回当前 FSM 的独立运行数据快照，包括实例 ID、转移 ID、名称、上一状态/事件、当前状态、处理中事件以及序列化后的 Payload：

```go
metadata := machine.Metadata()
fmt.Println(metadata.CurrentState, metadata.PreviousEvent)
```

每条 `TransitionTrace` 通过 `EntryMetadata` 和 `ExitMetadata` 分别保存事件进入 FSM 后、Handler 执行前的数据，以及事件处理完全结束后的最终数据。两份 Metadata 及其中的 Payload JSON 都是深拷贝，不会被后续状态或业务数据修改污染。

同一 FSM 不允许重入处理事件：Handler 内再次调用 `ProcessEvent`，或其他 goroutine 在事件尚未结束时调用，会立即得到 `ErrorEventProcessing`，不会阻塞死锁。Handler 执行期间若通过 `SetCurrentState` 改变状态，原转移会返回 `ErrorStateChanged`，避免覆盖外部状态修改。

## 异步事件队列

`SubmitEvent` 会把事件放入状态机内部队列。默认事件为高优先级、永不过期；同一优先级按提交顺序执行。调度优先级为：

1. 尚未开始执行的直接 `ProcessEvent` 或 `ProcessEventWithContext` 调用。
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

## FSM 数据持久化与重启恢复

配置模型位于 `github.com/swkings/fsm/config`，持久化实现位于 `github.com/swkings/fsm/store`。Store 是 FSM 的核心能力，不依赖可视化包：

```go
backend, err := store.New(config.Store{
    JSONL: config.JSONL{
        Directory: "data/fsm",
        Filename:  "orders.jsonl",
    },
})
if err != nil {
    return err
}
defer backend.Close()

payload := &OrderPayload{/* ... */}
machine := NewFSM("order", "pending", payload)
machine.SetTransitions(/* ... */)
if err := machine.BindStore(ctx, backend, "orders/order-1001"); err != nil {
    return err
}
```

`BindStore` 使用稳定的业务 key 查询最近一次成功记录：存在记录时恢复 InstanceID、TransitionID、当前/上一状态、上一事件、描述及 Payload；不存在记录时保存初始检查点。绑定后每次事件结束都会同步保存 FSM 数据、转移定义以及进入/退出 FSM 的 Trace。`SetPayload` 或 `SetCurrentState` 等非事件修改后可调用 `machine.Save(ctx)` 创建显式检查点。直接调用 `BindStore` 时 Store 生命周期由调用方管理；使用统一 `fsm.Server` 时由 Server 持有并关闭共享 Store。

未配置数据库时 `store.New(config.Store{})` 默认使用 `data/fsm/fsm.jsonl`。MySQL 和 Redis 通过同一配置模型选择：

```go
config.Store{
    Driver: config.StoreDriverMySQL,
    MySQL: config.MySQL{
        DSN:   "fsm:secret@tcp(mysql:3306)/fsm?parseTime=true",
        Table: "fsm_records",
    },
}

config.Store{
    Driver: config.StoreDriverRedis,
    Redis: config.Redis{
        Address:   "redis:6379",
        DB:        0,
        KeyPrefix: "fsm:store",
    },
}
```

Store 集成测试默认连接 `127.0.0.1:3306` 和 `127.0.0.1:6379`；默认连接不可用时会跳过。需要认证或使用其它地址时可显式配置，显式配置后的连接失败会令测试失败：

```bash
FSM_TEST_MYSQL_DSN='user:password@tcp(127.0.0.1:3306)/test?parseTime=true' \
FSM_TEST_REDIS_ADDRESS='127.0.0.1:6379' \
FSM_TEST_REDIS_PASSWORD='password' \
FSM_TEST_REDIS_DB='0' \
go test ./store -v
```

## 实时可视化

可视化能力位于独立的 `github.com/swkings/fsm/visualization` 包。业务进程和独立聚合服务使用同一个 `Server` 实现，但承担不同职责：

- 业务进程内创建一个顶层 `fsm.Server`，多个 FSM 可在任意模块通过 `Register` 或 `RegisterTo` 注册，并在状态变化后通过进程内订阅主动 Push。
- 顶层 Server 同时管理 Store、当前进程的 React 可视化前端和可选的 MCP endpoint，可直接通过配置的地址访问。
- 独立可视化服务不接收业务代码直接注册；它通过服务发现获得多个 Client Server 地址，再展开并 Pull 每个 Client Server 中的 FSM。

### 配置业务进程 Server

业务进程优先使用 `server.MustNewServer(config.Config)`，无需分别创建 Store、Replay 数据源和可视化 Server：

```go
s := server.MustNewServer(config.Config{
    Store: config.Store{
        JSONL: config.JSONL{Directory: "data/fsm"},
    },
    Visualization: config.Visualization{
        ServerAddress: "0.0.0.0:5005",
        TraceLimit:    100,
        PathPrefix:    "production",
    },
})
if err := s.Register(orderFSM, paymentFSM); err != nil {
    return err
}
services.Add(s)
```

独立聚合可视化服务与第三方 Client 适配仍可直接使用 `visualization.NewServerWithConfig`、`RegisterReplayStore` 和服务发现 API；这些底层接口保持兼容。

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

统一 Server 下无需直接调用 `visualization.RegisterClient`，改用 `s.Register(orderFSM, paymentFSM)` 或 `orderFSM.RegisterTo(s)`；Payload 的显示名称和目录规则保持不变。

`PathPrefix` 与 Payload 的 `Category()` 使用 `/` 组合，例如上例最终目录为 `production/orders/checkout`；两者都允许为空。Payload 对应方法返回空字符串时，显示名称回退到 FSM 名称，目录仅使用非空部分。

内置 `fsm.FSM` 已实现 `visualization.IPushClient`。注册时 Server 只读取一次完整 `Definition`；注册完成后，FSM 事件结束会将 `RealtimeData` 和增量 Trace 主动推送到 Client Server。取消注册 context、调用 `Unregister` 或关闭 Client Server 都会移除订阅。

第三方状态机可以实现相同接口。实现 `IPushClient` 时默认采用 Push；只实现 `IClient` 时默认采用 Pull。需要指定显示信息、目录、模式或 Pull 周期时，由客户端实现 `IRegistrationConfigProvider`：

```go
type IClient interface {
    VisualizationDefinition() Definition
    VisualizationSnapshot(context.Context) (RealtimeData, error)
}

type IPushClient interface {
    IClient
    SubscribeVisualization(PublishFunc) func()
}

type IRegistrationConfigProvider interface {
    VisualizationRegistrationConfig() RegistrationConfig
}
```

## MCP Server

MCP 协议实现、查询模型和 Streamable HTTP handler 位于独立的 `github.com/swkings/fsm/mcp` 包；`server` 包只负责按配置创建和管理其生命周期。

配置 `FSM.MCP.Enabled: true` 或填写 `FSM.MCP.ServerAddress` 后，统一 Server 会启动官方 Go SDK v1.7.0 的无状态 Streamable HTTP endpoint，支持 MCP `2026-07-28` 并向后协商旧协议：

```yaml
FSM:
  MCP:
    Enabled: true
    ServerAddress: 127.0.0.1:5006
    Endpoint: /mcp
    Name: order-fsm
    Version: v1.0.0
    MaxRequestBodyBytes: 4194304
```

MCP Server 暴露三个只读、幂等工具：

- `fsm_list`：按 ID、名称、显示名称、路径或当前状态筛选注册 FSM。
- `fsm_get`：查询指定 FSM 的完整状态、Payload、状态图、可用转移和 Trace。
- `fsm_traces`：按 FSM ID 查询最新 Trace，可使用 `limit` 限制数量。

启动后可通过 `s.MCPURL()` 获取完整 endpoint，通过 `s.VisualizationURL()` 获取可视化页面地址。

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

- 当前状态、Payload、Trace、全部状态转移和当前可接收事件；每条 Trace 可查看进入与退出 FSM 时的完整 Metadata。
- FSM 按注册 `path` 的 `/` 层级形成可聚合折叠的文件夹树。
- 左侧目录树支持调整宽度和折叠/展开；右侧 Inspector 支持折叠/展开，两侧状态均会记忆。
- 状态图作为全屏画布，右侧竖向工具栏提供放大、缩小、恢复默认视图和 PNG 导出；画布也支持连续拖动。
- 状态节点支持鼠标悬浮或键盘聚焦查看转移信息。
- Header 左侧展示当前 FSM 完整路径，最右侧按钮用于展开或折叠 Header 下方的右侧 Inspector；Inspector 顶部使用横向 Nav，目前提供 Payload。Active Event 面板通过自身标题栏折叠或展开，Trace 固定在底部状态栏并可展开完整记录。Header 的全局透明度会同步作用于左右侧栏以及 Active Event、Payload、Trace 等面板，默认 85%。
- 明暗主题切换、pull 采集错误以及动态注册/注销。

Header 中部提供 `Visualization` 与 `Replay` 两个版面。Replay 左侧栏按目录展示数据源中的 FSM，选择后可浏览该 FSM 的记录、播放状态变化并查看当时的状态图；Payload 与 Visualization 一样展示在可折叠的右侧 Inspector 中。设置页的“数据源发现”支持运行时挂载 JSONL 目录、MySQL 或 Redis Store。可视化服务只读取和管理这些数据源，不负责 FSM 落盘；FSM 持久化必须由业务侧创建 Store 并调用 `BindStore`。

Replay HTTP API 包括 `GET/POST /api/replay/sources`、`DELETE /api/replay/sources/{id}`、`GET /api/replay/records`、`GET /api/replay/records/{id}` 和 `DELETE /api/replay/records/{id}`。记录接口使用 `source` 查询参数选择数据源，并可用 `fsmId` 过滤状态机。

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
