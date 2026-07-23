package fsm

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

type (
	// FSMName 表示状态机名称，用于区分不同的状态机定义或实例。
	FSMName string
	// FSMState 表示状态机中的一个状态。
	FSMState string
	// FSMEvent 表示触发状态迁移的事件。
	FSMEvent string
)

// EventStateKey 由事件和事件发生前的状态组成，用于唯一定位一条迁移规则。
// 同一个事件可以在不同状态下对应不同目标状态，因此不能只使用 Event 作为索引。
type EventStateKey struct {
	// Event 是待处理的事件。
	Event FSMEvent
	// FromState 是处理该事件时要求状态机所处的状态。
	FromState FSMState
}

// String 返回事件与源状态拼接后的可读标识，主要用于错误信息和生成代码中的名称。
func (esk *EventStateKey) String() string {
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
	// ErrorEventStateKeyRepeated 构造“迁移规则重复注册”的错误。
	ErrorEventStateKeyRepeated = func(eventStateKey EventStateKey) error {
		return fmt.Errorf("event state key %v is repeated", eventStateKey.String())
	}
	// ErrorSubFSMInvalid 表示子状态机配置无效。
	ErrorSubFSMInvalid = fmt.Errorf("sub fsm invalid")
	// ErrorSubFSMSameAsFather 表示子状态机不能与父状态机相同。
	ErrorSubFSMSameAsFather = fmt.Errorf("sub fsm can not same as father fsm")
)

// TransitionItem 描述一条完整的状态迁移规则：FromState --Event/Handler--> ToState。
// Handler 可以为零值；此时事件仍会直接完成状态迁移。
type TransitionItem[T any] struct {
	// Event 是触发迁移的事件。
	Event FSMEvent
	// FromState 是迁移发生前的状态。
	FromState FSMState
	// ToState 是各阶段处理成功后提交的目标状态。
	ToState FSMState
	// Handler 包含迁移前检查、核心业务处理和迁移后处理三个阶段。
	Handler Handler[T]
}

// NewTransitionItem 创建一条状态迁移规则。
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

	// Handler 定义一次状态迁移的三阶段处理流程。
	// 执行顺序固定为 BeforeHandler、Handler、AfterHandler；nil 阶段会被直接跳过。
	Handler[T any] struct {
		// BeforeHandler 执行状态校验、权限检查或其他业务前置条件判断。
		// 返回错误时会立即中断本次迁移，后续阶段不会执行。
		BeforeHandler Actor[T]

		// Handler 执行本次迁移的核心业务逻辑。
		// 返回错误时状态机仍停留在原状态；业务数据回滚需由调用方自行处理。
		Handler Actor[T]

		// AfterHandler 执行保存派生数据、清理临时数据等收尾工作。
		// 当前实现会调用它，但不会向上传递其返回错误。
		AfterHandler Actor[T]
	}

	// FSM 是一个支持泛型业务载荷的有限状态机实例。
	// 迁移规则通过 SetTransitions 注册，运行阶段通过 ProcessEvent 串行处理事件。
	FSM[T any] struct {
		// TtId 是最近一次成功迁移的追踪标识；创建实例时会生成初始标识。
		TtId string
		// Name 是状态机名称。
		Name FSMName
		// Describe 是供调用方记录状态机用途的描述文本。
		Describe string
		// fromState 记录最近一次成功迁移前的状态。
		fromState FSMState
		// fromEvent 记录最近一次成功迁移使用的事件。
		fromEvent FSMEvent
		// processingEvent 记录当前正在处理的事件；成功迁移后恢复为空事件。
		processingEvent FSMEvent
		// currentState 是状态机当前已提交的状态。
		currentState FSMState
		// transitions 保存“事件+源状态”到目标状态的映射。
		transitions map[EventStateKey]FSMState
		// handlers 保存每条迁移规则对应的三阶段处理器。
		handlers map[EventStateKey]Handler[T]
		// eventSearch 反向索引每个状态下允许处理的事件列表。
		eventSearch map[FSMState][]FSMEvent
		// Payload 是与状态机绑定的业务数据；未传入载荷时为 nil。
		Payload *T
		// stateLock 保护 currentState 的直接读写。
		stateLock sync.RWMutex
		// eventLock 保证同一个状态机实例同一时间只处理一个事件。
		eventLock sync.Mutex
		// eventSearchLock 保护 eventSearch 的读写。
		eventSearchLock sync.RWMutex
	}
)

// NewFSM 创建状态机实例，并将 currentState 初始化为 initState。
// payloads 是可选参数；传入多个值时只使用第一个，未传入时 Payload 为 nil。
func NewFSM[T any](fsmName FSMName, initState FSMState, payloads ...*T) *FSM[T] {
	fsm := &FSM[T]{
		TtId:            uuid.NewString(),
		Name:            fsmName,
		fromState:       StateEmpty,
		fromEvent:       EventEmpty,
		currentState:    initState,
		processingEvent: EventEmpty,
		transitions:     make(map[EventStateKey]FSMState),
		handlers:        make(map[EventStateKey]Handler[T]),
		eventSearch:     make(map[FSMState][]FSMEvent),
		Payload: func() *T {
			if len(payloads) > 0 {
				return payloads[0]
			}
			return nil
		}(),
	}

	return fsm
}

// SetTransitions 批量注册状态迁移规则并返回 FSM，以便链式初始化。
// Event 和 FromState 相同的规则被视为重复规则，重复注册会触发 panic，避免已有规则被静默覆盖。
// 该方法预期在状态机开始处理事件前调用，不负责与 ProcessEvent 并发同步。
func (fsm *FSM[T]) SetTransitions(transitionItems ...TransitionItem[T]) *FSM[T] {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()

	fsm.eventLock.Lock()
	defer fsm.eventLock.Unlock()

	fsm.eventSearchLock.Lock()
	defer fsm.eventSearchLock.Unlock()

	for _, transitionItem := range transitionItems {
		eventStateKey := EventStateKey{
			Event:     transitionItem.Event,
			FromState: transitionItem.FromState,
		}
		if fsm.existEventStateKey(eventStateKey) {
			panic(ErrorEventStateKeyRepeated)
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
// 返回值是内部切片，调用方应将其视为只读数据，避免直接修改影响状态机配置。
func (fsm *FSM[T]) GetEventUnderState(state FSMState) []FSMEvent {
	fsm.eventSearchLock.RLock()
	defer fsm.eventSearchLock.RUnlock()

	return fsm.eventSearch[state]
}

// SetPayload 替换状态机绑定的业务载荷并返回 FSM，以便链式调用。
func (fsm *FSM[T]) SetPayload(payload *T) *FSM[T] {
	fsm.Payload = payload

	return fsm
}

// SetCurrentState 在加写锁后直接设置当前状态。
// 该方法不会执行 Handler、校验迁移规则或更新追踪信息，通常只用于状态恢复或初始化。
func (fsm *FSM[T]) SetCurrentState(state FSMState) {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()

	fsm.currentState = state
}

// GetCurrentState 在加读锁后返回当前已提交的状态。
func (fsm *FSM[T]) GetCurrentState() FSMState {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	return fsm.currentState
}

// ExistEventStateKey 判断指定的“事件+源状态”迁移规则是否已经注册。
func (fsm *FSM[T]) existEventStateKey(eventStateKey EventStateKey) bool {
	_, ok := fsm.transitions[eventStateKey]

	return ok
}

// MustGetTransitionStateAndHandler 返回指定迁移规则的目标状态和 Handler。
// 调用前应先通过 ExistEventStateKey 确认规则存在；规则不存在时会返回相应类型的零值。
func (fsm *FSM[T]) mustGetTransitionStateAndHandler(esk EventStateKey) (FSMState, Handler[T]) {
	state, handler := fsm.transitions[esk], fsm.handlers[esk]

	return state, handler
}

// ProcessEvent 在当前状态下处理事件，并返回本次迁移的完整追踪信息。
//
// 处理流程如下：
//
//  1. 串行化事件处理并记录待处理事件；
//  2. 查找“当前状态+事件”对应的迁移规则；
//  3. 按顺序执行 BeforeHandler、Handler 和 AfterHandler；
//  4. 全部处理成功后提交目标状态并生成新的迁移追踪标识。
//
// 规则不存在或 Handler 返回错误时会返回失败追踪，当前状态及追踪标识保持不变。
func (fsm *FSM[T]) ProcessEvent(event FSMEvent) (TransitionTrace, error) {
	fsm.eventLock.Lock()
	defer fsm.eventLock.Unlock()

	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	fsm.processingEvent = event
	nextTtId := uuid.NewString()
	transTrace := NewTransitionTrace(fsm.TtId, nextTtId, fsm.currentState, event).Start()

	eventStateKey := EventStateKey{
		Event:     event,
		FromState: fsm.currentState,
	}

	if !fsm.existEventStateKey(eventStateKey) {
		transTrace.SetFinalState(fsm.currentState).End()
		return transTrace.TrimPtr(), ErrorEventStateKeyInvalid(eventStateKey)
	}

	toState, handler := fsm.mustGetTransitionStateAndHandler(eventStateKey)
	transTrace.SetToState(toState)

	if err := handler.apply(fsm); err != nil {
		transTrace.SetFinalState(fsm.currentState).End(err)
		return transTrace.TrimPtr(), err
	}

	transTrace.SetFinalState(toState).TransitionSuccess().End()

	fsm.setEnvironment(
		event,
		toState,
		nextTtId,
	)

	return transTrace.TrimPtr(), nil
}

// apply 按固定顺序执行 Handler 的三个阶段。
// BeforeHandler 或 Handler 返回错误时立即中止后续阶段。为保持当前行为，
// AfterHandler 的返回值不会向上传递；收尾阶段如需处理错误，应在内部自行完成。
func (h Handler[T]) apply(
	fsm *FSM[T],
) error {
	if h.BeforeHandler != nil {
		err := h.BeforeHandler(fsm)
		if err != nil {
			return err
		}
	}

	if h.Handler != nil {
		err := h.Handler(fsm)
		if err != nil {
			return err
		}
	}

	if h.AfterHandler != nil {
		h.AfterHandler(fsm)
	}

	return nil
}

// setEnvironment 在迁移成功后一次性更新状态机运行环境。
// 它记录迁移前状态与事件，清除正在处理的事件，并提交目标状态和新的迁移追踪标识。
func (fsm *FSM[T]) setEnvironment(
	event FSMEvent,
	toState FSMState,
	ttId string,
) {
	fsm.fromState = fsm.currentState
	fsm.fromEvent = event
	fsm.processingEvent = EventEmpty

	fsm.currentState = toState
	fsm.TtId = ttId
}
