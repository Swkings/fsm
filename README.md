# FSM

这是一个支持泛型 Payload、三阶段 Handler、迁移追踪以及 Graphviz/Mermaid 可视化的有限状态机库。

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

## 可视化

- `DotString` 输出经过转义的 Graphviz DOT 文本。
- `VisualizeGraphviz` 直接调用本机 `dot`，不经过系统 Shell；运行环境需要安装 Graphviz。
- `VisualizeForMermaidWithGraphType` 输出 Mermaid 流程图或状态图。
- `VisualizeMermaid` 会安全覆盖目标文件并返回文件错误。
