package fsm

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	commonds "github.com/swkings/fsm/ds"
)

type (
	FSMName         = commonds.FSMName
	FSMState        = commonds.FSMState
	FSMEvent        = commonds.FSMEvent
	EventStateKey   = commonds.EventStateKey
	FSMMetadata     = commonds.Metadata
	TransitionTrace = commonds.TransitionTrace
)

var (
	// EventEmpty indicates that no event is currently being processed.
	EventEmpty = FSMEvent("")
	// StateEmpty indicates an absent or uninitialized state.
	StateEmpty = FSMState("")
)

// PayloadI is the business payload contract used by an FSM.
// Applications normally pass a pointer so handlers can mutate the same payload.
type PayloadI interface {
	JSON() ([]byte, error)
	DisplayName() string
	Category() string
}

// Actor is one handler stage. Params are the values passed to ProcessEvent.
type Actor[T PayloadI] func(*FSM[T], ...any) error

// Handler defines the fixed before, action, and after stages of a transition.
type Handler[T PayloadI] struct {
	BeforeHandler Actor[T]
	Handler       Actor[T]
	AfterHandler  Actor[T]
}

// TransitionItem describes one configured FromState --Event--> ToState rule.
type TransitionItem[T PayloadI] struct {
	Event     FSMEvent
	FromState FSMState
	ToState   FSMState
	Handler   Handler[T]
}

// NewTransitionItem creates one transition rule.
func NewTransitionItem[T PayloadI](event FSMEvent, fromState, toState FSMState, handler Handler[T]) TransitionItem[T] {
	return TransitionItem[T]{Event: event, FromState: fromState, ToState: toState, Handler: handler}
}

// // FSM is a finite state machine with a mutable application payload.
type FSM[T PayloadI] struct {
	// instanceID 是状态机实例的稳定标识，不随转移追踪标识变化。
	InstanceID string
	// TtId 是最近一次成功转移的追踪标识；创建实例时会生成初始标识。
	TtId string
	// Name 是状态机名称。
	Name FSMName
	// Describe 是供调用方记录状态机用途的描述文本。
	Describe string

	// Payload 是与状态机绑定的业务数据。
	Payload T

	// fromState 记录最近一次成功转移前的状态。
	fromState FSMState
	// fromEvent 记录最近一次成功转移使用的事件。
	fromEvent FSMEvent
	// processingEvent 记录当前正在处理的事件；完成后恢复为空事件。
	processingEvent FSMEvent
	// processingContext 向 Handler 暴露同步或异步事件的取消信号。
	processingContext context.Context
	// currentState 是状态机当前已提交的状态。
	currentState FSMState
	// transitions 保存“事件+源状态”到目标状态的映射。
	transitions map[EventStateKey]FSMState
	// handlers 保存每条转移规则对应的三阶段处理器。
	handlers map[EventStateKey]Handler[T]
	// eventSearch 反向索引每个状态下允许处理的事件列表。
	eventSearch map[FSMState][]FSMEvent

	// stateLock 保护运行状态、转移追踪标识和处理中的事件。
	stateLock sync.RWMutex
	// eventLock 保证同一个状态机实例同一时间只处理一个事件。
	eventLock sync.Mutex
	// eventScheduleLock 原子协调直接调用和异步队列对事件执行权的竞争。
	eventScheduleLock sync.Mutex
	// directEventPending 让异步 worker 为已经到达的直接 ProcessEvent 调用让行。
	directEventPending atomic.Int64
	// eventQueue 保存并调度异步提交的高、低优先级事件。
	eventQueue *eventQueue
	// configLock 统一保护 transitions、handlers 和 eventSearch，确保转移配置一致。
	configLock sync.RWMutex
	// runtimeObserverLock 保护可视化 Server 注册的运行时观察器。
	runtimeObserverLock sync.RWMutex
	// runtimeObservers 在事件完成后接收 trace 或快照更新通知。
	runtimeObservers map[string]func(*TransitionTrace)
	// persistence 保存可选的 FSM 持久化绑定；具体后端由 store 包提供。
	persistence persistenceBinding
}

// NewFSM creates an FSM in initState with the supplied payload.
func NewFSM[T PayloadI](fsmName FSMName, initState FSMState, payload T) *FSM[T] {
	return &FSM[T]{
		InstanceID: uuid.NewString(), TtId: uuid.NewString(), Name: fsmName,
		fromState: StateEmpty, fromEvent: EventEmpty, currentState: initState,
		processingEvent: EventEmpty, processingContext: context.Background(),
		transitions: make(map[EventStateKey]FSMState), handlers: make(map[EventStateKey]Handler[T]),
		eventSearch: make(map[FSMState][]FSMEvent), eventQueue: newEventQueue(),
		runtimeObservers: make(map[string]func(*TransitionTrace)), Payload: payload,
	}
}
