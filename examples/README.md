# Examples

示例均使用仓库当前版本的 `github.com/swkings/fsm`，可在仓库根目录直接运行。

## 基础状态转移

演示创建 FSM 时传入 `&Payload{}`，并在 `BeforeHandler`、`Handler`、`AfterHandler` 中共享修改同一份 Payload，以及查看 `TransitionTrace`：

```bash
go run ./examples/basic
```

## YAML 自动生成角色 FSM

[`generated/player.fsm`](./generated/player.fsm) 定义了玩家的移动、跳跃、轻重攻击、受击、死亡和复活状态。`generated/player` 下的工厂、元数据及 32 条转移 Handler 均由该 YAML 生成；生成工厂接收 `*types.Payload`，调用方通过 `NewFSMPlayerAction(&types.Payload{...})` 创建状态机。状态和事件分别通过 `types.StateEnum`、`types.EventEnum` 访问。Handler 文件采用 `s_STATE_e_EVENT.go` 命名，内部使用 `_Before`、`_In`、`_After` 三阶段方法。轻重攻击通过 `ProcessEvent` 携带 `AttackParams` 并扣除指定目标血量，角色死亡后可通过 `REVIVE` 回到 `IDLE`。

运行示例：

```bash
go run ./examples/generated
```

示例从 `generated/config.yaml` 读取 Client Server 地址、Trace 上限和目录前缀；FSM 显示名称与相对目录分别由 Payload 的 `DisplayName()`、`Category()` 提供，启动后可通过配置地址访问可视化前端。运行期间每隔随机 3～5 秒从当前状态的合法事件中选择一个执行；攻击事件会随机生成伤害参数，死亡后下一次合法事件为复活。可通过 `-config` 指定其他配置文件。运行时配置与 `player.fsm` 生成描述相互独立，不进入自动生成代码。

重新生成工厂和模板代码：

```bash
go run gen/main.go \
  -fsm ./examples/generated/player.fsm \
  -tpl ./gen/template \
  -out ./examples/generated
```

默认不会覆盖已经实现的 `payload.go` 和各 Handler 文件。只有希望重置这些可编辑文件时才添加 `-c`。

## 实时可视化 Server

演示通过 `NewServer(config)` 创建业务进程内全局单例可视化 Server，并在不同位置使用无需注册配置的 `RegisterClient(ctx, client)` 注册多个 FSM。显示名称和相对目录由 Client 或 FSM Payload 提供。注册时一次性提交完整 Definition；本库 FSM 实现 `PushClient`，状态变化后会主动推送 RealtimeData 与 Trace。第三方状态机也可以作为 pull Client 注册到同一 Server。

独立可视化服务还支持通过 `RegisterClientServerDiscovery` 或 Web Header 中的服务发现配置，发现多个业务进程的 Client Server，再展开并持续 pull 各进程内注册的 FSM；发现地址也可以直接填写单个 Client Server 的基础 URL。

React Web 页面以可拖动状态图作为全屏画布；Visualization 与 Replay 都使用 FSM 左侧栏和 Payload 右侧 Inspector，Replay 左侧栏用于切换已持久化的其它 FSM。Header 左侧展示当前 FSM 完整路径，最右侧按钮控制 Header 下方的右侧 Inspector。Active Event 面板通过自身标题栏折叠或展开，Trace 位于底部可展开状态栏。Header 的全局透明度会同步作用于左右侧栏以及 Active Event、Payload、Trace 等面板；右侧竖向画布工具栏提供缩放、复位和 PNG 导出，同时支持文件夹聚合搜索、节点悬浮信息、侧栏折叠记忆和明暗主题切换：

```bash
go run ./examples/server
```

程序会输出本机访问 URL，按 `Ctrl+C` 退出。

## 服务发现与远程 Pull

演示三段式架构：业务进程启动全局 Client Server 并注册 FSM；HTTP 注册中心只返回 Client Server 地址；独立可视化服务发现 Client Server、展开其中的多个 FSM 并持续 pull。程序会同时输出客户端前端和聚合前端地址：

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
