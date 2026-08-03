package fsm

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	commonasync "github.com/swkings/fsm/common/async"
	"github.com/swkings/fsm/ds"
)

// PayloadI is the business payload contract used by an FSM.
// Applications normally pass a pointer so handlers can mutate the same payload.
type PayloadI interface {
	JSON() ([]byte, error)
	DisplayName() string
	Category() string
}

// TransitionItem describes one configured FromState --Event--> ToState rule.
type TransitionItem[T PayloadI] struct {
	Event     Event
	FromState State
	ToState   State
	Handler   Handler[T]
}

// NewTransitionItem creates one transition rule.
func NewTransitionItem[T PayloadI](
	event Event,
	fromState, toState State,
	handler Handler[T],
) TransitionItem[T] {
	return TransitionItem[T]{
		Event:     event,
		FromState: fromState,
		ToState:   toState,
		Handler:   handler,
	}
}

// FSM is a finite state machine with a mutable application payload.
type FSM[T PayloadI] struct {
	// instanceID 是状态机实例的稳定标识，不随转移追踪标识变化。
	InstanceID string
	// TtId 是最近一次成功转移的追踪标识；创建实例时会生成初始标识。
	TtId string
	// Name 是状态机名称。
	Name Name
	// Describe 是供调用方记录状态机用途的描述文本。
	Describe string

	// Payload 是与状态机绑定的业务数据。
	Payload T

	// fromState 记录最近一次成功转移前的状态。
	fromState State
	// fromEvent 记录最近一次成功转移使用的事件。
	fromEvent Event
	// processingEvent 记录当前正在处理的事件；完成后恢复为空事件。
	processingEvent Event
	// processingContext 向 Handler 暴露同步或异步事件的取消信号。
	processingContext context.Context
	// currentState 是状态机当前已提交的状态。
	currentState State
	// transitions 保存“事件+源状态”到目标状态的映射。
	transitions map[EventStateKey]State
	// handlers 保存每条转移规则对应的三阶段处理器。
	handlers map[EventStateKey]Handler[T]
	// eventSearch 反向索引每个状态下允许处理的事件列表。
	eventSearch map[State][]Event

	// stateLock 保护运行状态、转移追踪标识和处理中的事件。
	stateLock sync.RWMutex
	// eventLock 保证同一个状态机实例同一时间只处理一个事件。
	eventLock sync.Mutex
	// eventScheduleLock 原子协调直接调用和异步队列对事件执行权的竞争。
	eventScheduleLock sync.Mutex
	// directEventPending 让异步 worker 为已经到达的直接 ProcessEvent 调用让行。
	directEventPending atomic.Int64
	// eventQueue 保存并调度异步提交的高、低优先级事件。
	eventQueue *commonasync.Queue[*queuedEvent]
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
func NewFSM[T PayloadI](fsmName Name, initState State, payload T) *FSM[T] {
	return &FSM[T]{
		InstanceID:        uuid.NewString(),
		TtId:              uuid.NewString(),
		Name:              fsmName,
		fromState:         StateEmpty,
		fromEvent:         EventEmpty,
		currentState:      initState,
		processingEvent:   EventEmpty,
		processingContext: context.Background(),
		transitions:       make(map[EventStateKey]State),
		handlers:          make(map[EventStateKey]Handler[T]),
		eventSearch:       make(map[State][]Event),
		eventQueue:        commonasync.NewQueue[*queuedEvent](),
		runtimeObservers:  make(map[string]func(*TransitionTrace)),
		Payload:           payload,
	}
}

// ProcessEvent processes an event with a background context.
func (fsm *FSM[T]) ProcessEvent(
	event Event,
	params ...any,
) (
	TransitionTrace,
	error,
) {
	return fsm.ProcessEventWithContext(
		context.Background(),
		event,
		params...,
	)
}

// ProcessEventWithContext processes an event and propagates cancellation between handler stages.
func (fsm *FSM[T]) ProcessEventWithContext(
	ctx context.Context,
	event Event,
	params ...any,
) (
	TransitionTrace,
	error,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return fsm.rejectedRuntimeEventTrace(event, err), err
	}

	fsm.directEventPending.Add(1)
	fsm.eventScheduleLock.Lock()
	acquired := fsm.eventLock.TryLock()
	fsm.eventScheduleLock.Unlock()
	fsm.directEventPending.Add(-1)
	if !acquired {
		return fsm.rejectedRuntimeEventTrace(
			event, ErrorEventProcessing,
		), ErrorEventProcessing
	}
	defer fsm.releaseEventExecution()

	return fsm.processEventLocked(ctx, event, params...)
}

func (fsm *FSM[T]) processEventLocked(
	ctx context.Context,
	event Event,
	params ...any,
) (
	TransitionTrace,
	error,
) {
	trace, err := fsm.processEventLockedCore(ctx, event, params...)
	persistErr := fsm.persistTrace(context.Background(), trace)
	fsm.notifyRuntimeObservers(&trace)
	if err == nil && persistErr != nil {
		return trace, persistErr
	}

	return trace, err
}

func (fsm *FSM[T]) processEventLockedCore(
	ctx context.Context,
	event Event,
	params ...any,
) (
	result TransitionTrace,
	resultErr error,
) {
	if err := ctx.Err(); err != nil {
		return fsm.rejectedEventTrace(event, err), err
	}
	fromState, previousID := fsm.beginProcessingEvent(ctx, event)
	fsm.notifyRuntimeObservers(nil)
	nextID := uuid.NewString()
	trace := ds.NewTransitionTrace(previousID, nextID, fromState, event).Start()
	trace.SetEntryMetadata(fsm.Metadata())
	defer func() {
		fsm.clearProcessingEvent()
		trace.SetExitMetadata(fsm.Metadata())
		if resultErr != nil {
			trace.End(resultErr)
		} else {
			trace.End()
		}
		result = trace.TrimPtr()
	}()
	key := EventStateKey{Event: event, FromState: fromState}
	toState, handler, exists := fsm.getTransition(key)
	if !exists {
		resultErr = ErrorEventStateKeyInvalid(key)
		trace.SetFinalState(fsm.GetCurrentState()).TransitionFail()

		return
	}
	trace.SetToState(toState)
	if resultErr = handler.apply(ctx, fsm, params...); resultErr != nil {
		trace.SetFinalState(fsm.GetCurrentState()).TransitionFail()

		return
	}
	if resultErr = ctx.Err(); resultErr != nil {
		trace.SetFinalState(fsm.GetCurrentState()).TransitionFail()

		return
	}
	var finalState State
	finalState, resultErr = fsm.commitTransition(
		fromState, event, toState, nextID,
	)
	if resultErr != nil {
		trace.SetFinalState(finalState).TransitionFail()

		return
	}
	trace.SetFinalState(finalState).TransitionSuccess()

	return
}

func (fsm *FSM[T]) releaseEventExecution() {
	fsm.eventLock.Unlock()
	if fsm.eventQueue != nil {
		fsm.eventQueue.Signal()
	}
}
