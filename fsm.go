package fsm

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

type (
	FSMName  string
	FSMState string
	FSMEvent string
)

type EventStateKey struct {
	Event     FSMEvent
	FromState FSMState
}

func (esk *EventStateKey) String() string {
	return string(esk.Event) + "_" + string(esk.FromState)
}

var (
	EventEmpty                = FSMEvent("")
	StateEmpty                = FSMState("")
	ErrorEventStateKeyInvalid = func(eventStateKey EventStateKey) error {
		return fmt.Errorf("event state key %v not exist", eventStateKey.String())
	}
	ErrorEventStateKeyRepeated = func(eventStateKey EventStateKey) error {
		return fmt.Errorf("event state key %v is repeated", eventStateKey.String())
	}
	ErrorSubFSMInvalid      = fmt.Errorf("sub fsm invalid")
	ErrorSubFSMSameAsFather = fmt.Errorf("sub fsm can not same as father fsm")
)

type TransitionItem[T any] struct {
	Event     FSMEvent
	FromState FSMState
	ToState   FSMState
	Handler   Handler[T]
}

func NewTransitionItem[T any](event FSMEvent, fromState FSMState, toState FSMState, handler Handler[T]) TransitionItem[T] {
	item := TransitionItem[T]{
		Event:     event,
		FromState: fromState,
		ToState:   toState,
		Handler:   handler,
	}

	return item
}

type (
	Actor[T any] func(*FSM[T]) error

	Handler[T any] struct {
		// BeforeHandler check state etc.
		//	- can not change the current state
		// 	- if check fail, should exit all handler function
		BeforeHandler Actor[T]

		// Handler real logic etc.
		// 	- finish state change logic
		// 	- if logic abort, should recover the data of ahead exec Handler, then exit
		Handler Actor[T]

		// AfterHandler save state, clear data etc.
		AfterHandler Actor[T]
	}

	FSM[T any] struct {
		TtId            string
		Name            FSMName
		Describe        string
		fromState       FSMState
		fromEvent       FSMEvent
		processingEvent FSMEvent
		currentState    FSMState
		// map[EventFromStateKey]ToState, FromState --Event-> ToState
		transitions map[EventStateKey]FSMState
		// map[EventFromStateKey]Handler, FromState --Handler(Event)-> ToState
		handlers    map[EventStateKey]Handler[T]
		eventSearch map[FSMState][]FSMEvent
		Payload     *T
		stateLock   sync.RWMutex
		eventLock   sync.Mutex
	}
)

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

func (fsm *FSM[T]) SetTransitions(transitionItems ...TransitionItem[T]) *FSM[T] {
	for _, transitionItem := range transitionItems {
		eventStateKey := EventStateKey{
			Event:     transitionItem.Event,
			FromState: transitionItem.FromState,
		}
		if fsm.ExistEventStateKey(eventStateKey) {
			panic(ErrorEventStateKeyRepeated)
		}
		fsm.transitions[eventStateKey] = transitionItem.ToState
		fsm.handlers[eventStateKey] = transitionItem.Handler
		fsm.setEventSearch(eventStateKey)
	}

	return fsm
}

func (fsm *FSM[T]) setEventSearch(eventStateKey EventStateKey) {
	if _, ok := fsm.eventSearch[eventStateKey.FromState]; !ok {
		fsm.eventSearch[eventStateKey.FromState] = []FSMEvent{}
	}
	fsm.eventSearch[eventStateKey.FromState] = append(fsm.eventSearch[eventStateKey.FromState], eventStateKey.Event)
}

func (fsm *FSM[T]) GetEventUnderState(state FSMState) []FSMEvent {
	return fsm.eventSearch[state]
}

func (fsm *FSM[T]) SetPayload(payload *T) *FSM[T] {
	fsm.Payload = payload

	return fsm
}

func (fsm *FSM[T]) SetCurrentState(state FSMState) {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()

	fsm.currentState = state
}

func (fsm *FSM[T]) GetCurrentState() FSMState {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	return fsm.currentState
}

func (fsm *FSM[T]) ExistEventStateKey(eventStateKey EventStateKey) bool {
	_, ok := fsm.transitions[eventStateKey]

	return ok
}

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

	if !fsm.ExistEventStateKey(eventStateKey) {
		transTrace.SetFinalState(fsm.currentState).End()
		return transTrace.TrimPtr(), ErrorEventStateKeyInvalid(eventStateKey)
	}

	toState, handler := fsm.transitions[eventStateKey], fsm.handlers[eventStateKey]
	transTrace.SetToState(toState)

	if handler.BeforeHandler != nil {
		err := handler.BeforeHandler(fsm)
		if err != nil {
			transTrace.SetFinalState(fsm.currentState).End(err)
			return transTrace.TrimPtr(), err
		}
	}

	if handler.Handler != nil {
		err := handler.Handler(fsm)
		if err != nil {
			transTrace.SetFinalState(fsm.currentState).End(err)
			return transTrace.TrimPtr(), err
		}
	}

	if handler.AfterHandler != nil {
		handler.AfterHandler(fsm)
	}

	transTrace.SetFinalState(toState).TransitionSuccess().End()

	fsm.fromState = fsm.currentState
	fsm.fromEvent = event
	fsm.processingEvent = EventEmpty

	fsm.currentState = toState
	fsm.TtId = nextTtId

	return transTrace.TrimPtr(), nil
}
