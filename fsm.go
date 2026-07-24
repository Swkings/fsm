package fsm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

type (
	// FSMName 表示状态机名称，用于区分不同的状态机定义或实例。
	FSMName string
	// FSMState 表示状态机中的一个状态。
	FSMState string
	// FSMEvent 表示触发状态转移的事件。
	FSMEvent string
)

// EventStateKey 由事件和事件发生前的状态组成，用于唯一定位一条转移规则。
// 同一个事件可以在不同状态下对应不同目标状态，因此不能只使用 Event 作为索引。
type EventStateKey struct {
	// Event 是待处理的事件。
	Event FSMEvent
	// FromState 是处理该事件时要求状态机所处的状态。
	FromState FSMState
}

// String 返回事件与源状态拼接后的可读标识，主要用于错误信息和生成代码中的名称。
func (esk EventStateKey) String() string {
	return string(esk.Event) + "_" + string(esk.FromState)
}

var (
	// EventEmpty 表示当前没有正在处理或已经记录的事件。
	EventEmpty = FSMEvent("")
	// StateEmpty 表示状态不存在或尚未初始化。
	StateEmpty = FSMState("")
	// ErrorEventStateKeyInvalid 构造“当前状态下不存在指定事件”的错误。
	ErrorEventStateKeyInvalid = func(eventStateKey EventStateKey) error {
		return fmt.Errorf("event state key %v not exist", eventStateKey.String())
	}
	// ErrorEventStateKeyRepeated 构造“转移规则重复注册”的错误。
	ErrorEventStateKeyRepeated = func(eventStateKey EventStateKey) error {
		return fmt.Errorf("event state key %v is repeated", eventStateKey.String())
	}
	// ErrorSubFSMInvalid 表示子状态机配置无效。
	ErrorSubFSMInvalid = errors.New("sub fsm invalid")
	// ErrorSubFSMSameAsFather 表示子状态机不能与父状态机相同。
	ErrorSubFSMSameAsFather = errors.New("sub fsm can not same as father fsm")
	// ErrorEventProcessing 表示状态机正在处理另一个事件，不能重入。
	ErrorEventProcessing = errors.New("fsm event is processing")
	// ErrorStateChanged 表示 Handler 执行期间状态被外部修改，转移无法安全提交。
	ErrorStateChanged = errors.New("fsm state changed during event processing")
)

// TransitionItem 描述一条完整的状态转移规则：FromState --Event/Handler--> ToState。
// Handler 可以为零值；此时事件仍会直接完成状态转移。
type TransitionItem[T any] struct {
	// Event 是触发转移的事件。
	Event FSMEvent
	// FromState 是转移发生前的状态。
	FromState FSMState
	// ToState 是各阶段处理成功后提交的目标状态。
	ToState FSMState
	// Handler 包含转移前检查、核心业务处理和转移后处理三个阶段。
	Handler Handler[T]
}

// NewTransitionItem 创建一条状态转移规则。
func NewTransitionItem[T any](
	event FSMEvent,
	fromState FSMState,
	toState FSMState,
	handler Handler[T],
) TransitionItem[T] {
	item := TransitionItem[T]{
		Event:     event,
		FromState: fromState,
		ToState:   toState,
		Handler:   handler,
	}

	return item
}

type (
	// Actor 是 Handler 各执行阶段的函数类型，可通过 FSM 访问状态机和业务载荷。
	Actor[T any] func(*FSM[T]) error

	// Handler 定义一次状态转移的三阶段处理流程。
	// 执行顺序固定为 BeforeHandler、Handler、AfterHandler；nil 阶段会被直接跳过。
	Handler[T any] struct {
		// BeforeHandler 执行状态校验、权限检查或其他业务前置条件判断。
		// 返回错误时会立即中断本次转移，后续阶段不会执行。
		BeforeHandler Actor[T]

		// Handler 执行本次转移的核心业务逻辑。
		// 返回错误时状态机仍停留在原状态；业务数据回滚需由调用方自行处理。
		Handler Actor[T]

		// AfterHandler 执行保存派生数据、清理临时数据等收尾工作。
		// 返回错误时本次转移失败，目标状态不会提交。
		AfterHandler Actor[T]
	}

	// FSM 是一个支持泛型业务载荷的有限状态机实例。
	// 转移规则通过 SetTransitions 注册，运行阶段通过 ProcessEvent 串行处理事件。
	FSM[T any] struct {
		// instanceID 是状态机实例的稳定标识，不随转移追踪标识变化。
		instanceID string
		// TtId 是最近一次成功转移的追踪标识；创建实例时会生成初始标识。
		TtId string
		// Name 是状态机名称。
		Name FSMName
		// Describe 是供调用方记录状态机用途的描述文本。
		Describe string
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
		// Payload 是与状态机绑定的业务数据；未传入载荷时为 nil。
		Payload *T
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
	}
)

// NewFSM 创建状态机实例，并将 currentState 初始化为 initState。
// payloads 是可选参数；传入多个值时只使用第一个，未传入时 Payload 为 nil。
func NewFSM[T any](fsmName FSMName, initState FSMState, payloads ...*T) *FSM[T] {
	fsm := &FSM[T]{
		instanceID:        uuid.NewString(),
		TtId:              uuid.NewString(),
		Name:              fsmName,
		fromState:         StateEmpty,
		fromEvent:         EventEmpty,
		currentState:      initState,
		processingEvent:   EventEmpty,
		processingContext: context.Background(),
		transitions:       make(map[EventStateKey]FSMState),
		handlers:          make(map[EventStateKey]Handler[T]),
		eventSearch:       make(map[FSMState][]FSMEvent),
		eventQueue:        newEventQueue(),
		runtimeObservers:  make(map[string]func(*TransitionTrace)),
		Payload: func() *T {
			if len(payloads) > 0 {
				return payloads[0]
			}
			return nil
		}(),
	}

	return fsm
}

// SetTransitions 批量注册状态转移规则并返回 FSM，以便链式初始化。
// Event 和 FromState 相同的规则被视为重复规则，重复注册会触发 panic，避免已有规则被静默覆盖。
// 注册过程使用独立配置锁，整批规则会先校验再原子提交，可与事件处理和可视化并发调用。
func (fsm *FSM[T]) SetTransitions(transitionItems ...TransitionItem[T]) *FSM[T] {
	fsm.configLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.configLock.Unlock()

	seen := make(map[EventStateKey]struct{}, len(transitionItems))
	for _, transitionItem := range transitionItems {
		eventStateKey := EventStateKey{
			Event:     transitionItem.Event,
			FromState: transitionItem.FromState,
		}
		if _, exists := fsm.transitions[eventStateKey]; exists {
			panic(ErrorEventStateKeyRepeated(eventStateKey))
		}
		if _, repeated := seen[eventStateKey]; repeated {
			panic(ErrorEventStateKeyRepeated(eventStateKey))
		}
		seen[eventStateKey] = struct{}{}
	}

	for _, transitionItem := range transitionItems {
		eventStateKey := EventStateKey{
			Event:     transitionItem.Event,
			FromState: transitionItem.FromState,
		}
		fsm.transitions[eventStateKey] = transitionItem.ToState
		fsm.handlers[eventStateKey] = transitionItem.Handler
		fsm.setEventSearch(eventStateKey)
	}

	return fsm
}

// setEventSearch 将事件加入源状态的可用事件索引。
func (fsm *FSM[T]) setEventSearch(eventStateKey EventStateKey) {
	if _, ok := fsm.eventSearch[eventStateKey.FromState]; !ok {
		fsm.eventSearch[eventStateKey.FromState] = []FSMEvent{}
	}
	fsm.eventSearch[eventStateKey.FromState] = append(fsm.eventSearch[eventStateKey.FromState], eventStateKey.Event)
}

// GetEventUnderState 返回指定状态下已注册的事件列表。
// 返回值是内部事件列表的副本，调用方可以安全修改。
func (fsm *FSM[T]) GetEventUnderState(state FSMState) []FSMEvent {
	fsm.configLock.RLock()
	defer fsm.configLock.RUnlock()

	return append([]FSMEvent(nil), fsm.eventSearch[state]...)
}

// SetPayload 在状态锁保护下替换状态机绑定的业务载荷并返回 FSM，以便链式调用。
// Payload 指向的业务对象仍由调用方负责同步；Handler 失败时不会自动回滚其内部修改。
func (fsm *FSM[T]) SetPayload(payload *T) *FSM[T] {
	fsm.stateLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.stateLock.Unlock()

	fsm.Payload = payload

	return fsm
}

// SetCurrentState 在加写锁后直接设置当前状态。
// 该方法不会执行 Handler、校验转移规则或更新追踪信息，通常只用于状态恢复或初始化。
func (fsm *FSM[T]) SetCurrentState(state FSMState) {
	fsm.stateLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.stateLock.Unlock()

	fsm.currentState = state
}

// GetCurrentState 在加读锁后返回当前已提交的状态。
func (fsm *FSM[T]) GetCurrentState() FSMState {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	return fsm.currentState
}

// GetEventContext 返回当前事件的 context，供 Handler 监听提交方取消信号。
// fire-and-forget 事件或没有事件正在处理时返回 context.Background；排队过期时间不会进入该 context。
func (fsm *FSM[T]) GetEventContext() context.Context {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()
	if fsm.processingContext == nil {
		return context.Background()
	}
	return fsm.processingContext
}

// getTransition 返回转移规则的目标状态、Handler 及存在标记。
func (fsm *FSM[T]) getTransition(eventStateKey EventStateKey) (FSMState, Handler[T], bool) {
	fsm.configLock.RLock()
	defer fsm.configLock.RUnlock()

	state, exists := fsm.transitions[eventStateKey]
	return state, fsm.handlers[eventStateKey], exists
}

// visualizationSnapshot 复制可视化所需的转移表和当前状态，避免持锁执行格式化。
func (fsm *FSM[T]) visualizationSnapshot() (map[EventStateKey]FSMState, FSMState) {
	fsm.configLock.RLock()
	transitions := make(map[EventStateKey]FSMState, len(fsm.transitions))
	for key, state := range fsm.transitions {
		transitions[key] = state
	}
	fsm.configLock.RUnlock()

	currentState, _ := fsm.stateSnapshot()
	return transitions, currentState
}

// ProcessEvent 在当前状态下处理事件，并返回本次转移的完整追踪信息。
//
// 处理流程如下：
//
//  1. 尝试取得事件处理权；状态机忙碌或发生重入时立即返回 ErrorEventProcessing；
//  2. 查找“当前状态+事件”对应的转移规则；
//  3. 按顺序执行 BeforeHandler、Handler 和 AfterHandler；
//  4. 全部处理成功后提交目标状态并生成新的转移追踪标识。
//
// 规则不存在或 Handler 返回错误时会返回失败追踪，当前状态及追踪标识保持不变。
func (fsm *FSM[T]) ProcessEvent(event FSMEvent) (TransitionTrace, error) {
	return fsm.ProcessEventContext(context.Background(), event)
}

// ProcessEventContext 直接处理事件，并允许 context 在 Handler 阶段之间取消转移。
// 直接调用拥有最高调度优先级；若另一个事件已经开始执行，则立即返回 ErrorEventProcessing。
func (fsm *FSM[T]) ProcessEventContext(ctx context.Context, event FSMEvent) (TransitionTrace, error) {
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
		err := ErrorEventProcessing
		return fsm.rejectedRuntimeEventTrace(event, err), err
	}
	defer fsm.releaseEventExecution()

	return fsm.processEventLocked(ctx, event)
}

// processEventLocked 在调用方已经取得事件执行权后完成转移。
func (fsm *FSM[T]) processEventLocked(ctx context.Context, event FSMEvent) (TransitionTrace, error) {
	trace, err := fsm.processEventLockedCore(ctx, event)
	fsm.notifyRuntimeObservers(&trace)
	return trace, err
}

// processEventLockedCore 执行实际转移；调用方通过 processEventLocked 统一发布运行时通知。
func (fsm *FSM[T]) processEventLockedCore(ctx context.Context, event FSMEvent) (TransitionTrace, error) {
	if err := ctx.Err(); err != nil {
		return fsm.rejectedEventTrace(event, err), err
	}

	fromState, prevTtID := fsm.beginProcessingEvent(ctx, event)
	// 在 Handler 执行前发布 processingEvent，使可视化页面能够动态高亮正在执行的转移。
	fsm.notifyRuntimeObservers(nil)
	defer fsm.clearProcessingEvent()

	nextTtID := uuid.NewString()
	trace := NewTransitionTrace(prevTtID, nextTtID, fromState, event).Start()
	eventStateKey := EventStateKey{Event: event, FromState: fromState}

	toState, handler, exists := fsm.getTransition(eventStateKey)
	if !exists {
		err := ErrorEventStateKeyInvalid(eventStateKey)
		trace.SetFinalState(fsm.GetCurrentState()).TransitionFail().End(err)
		return trace.TrimPtr(), err
	}
	trace.SetToState(toState)

	if err := handler.apply(ctx, fsm); err != nil {
		trace.SetFinalState(fsm.GetCurrentState()).TransitionFail().End(err)
		return trace.TrimPtr(), err
	}
	if err := ctx.Err(); err != nil {
		trace.SetFinalState(fsm.GetCurrentState()).TransitionFail().End(err)
		return trace.TrimPtr(), err
	}

	finalState, err := fsm.commitTransition(fromState, event, toState, nextTtID)
	if err != nil {
		trace.SetFinalState(finalState).TransitionFail().End(err)
		return trace.TrimPtr(), err
	}

	trace.SetFinalState(finalState).TransitionSuccess().End()
	return trace.TrimPtr(), nil
}

// releaseEventExecution 释放执行权并唤醒等待调度的异步队列。
func (fsm *FSM[T]) releaseEventExecution() {
	fsm.eventLock.Unlock()
	if fsm.eventQueue != nil {
		fsm.eventQueue.signal()
	}
}

// stateSnapshot 返回当前状态和最近一次成功转移标识的一致快照。
func (fsm *FSM[T]) stateSnapshot() (FSMState, string) {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	return fsm.currentState, fsm.TtId
}

// beginProcessingEvent 记录正在处理的事件，并返回转移开始前的状态快照。
func (fsm *FSM[T]) beginProcessingEvent(ctx context.Context, event FSMEvent) (FSMState, string) {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()

	fsm.processingEvent = event
	fsm.processingContext = ctx
	return fsm.currentState, fsm.TtId
}

// clearProcessingEvent 确保所有成功或失败路径都会清理正在处理的事件。
func (fsm *FSM[T]) clearProcessingEvent() {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()

	fsm.processingEvent = EventEmpty
	fsm.processingContext = context.Background()
}

// commitTransition 仅在状态未被外部修改时提交转移结果。
func (fsm *FSM[T]) commitTransition(
	fromState FSMState,
	event FSMEvent,
	toState FSMState,
	ttID string,
) (FSMState, error) {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()

	if fsm.currentState != fromState {
		return fsm.currentState, ErrorStateChanged
	}

	fsm.fromState = fromState
	fsm.fromEvent = event
	fsm.processingEvent = EventEmpty
	fsm.currentState = toState
	fsm.TtId = ttID

	return fsm.currentState, nil
}

// apply 按固定顺序执行 Handler 的三个阶段。
// 任一阶段返回错误都会立即终止处理并向上传递，目标状态不会提交。
func (h Handler[T]) apply(ctx context.Context, fsm *FSM[T]) error {
	stages := []Actor[T]{h.BeforeHandler, h.Handler, h.AfterHandler}
	for _, stage := range stages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if stage == nil {
			continue
		}
		if err := stage(fsm); err != nil {
			return err
		}
	}
	return ctx.Err()
}
