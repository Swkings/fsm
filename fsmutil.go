package fsm

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	fsmserver "github.com/swkings/fsm/server"
)

// Register registers the FSM with a configuration-driven server.
func (fsm *FSM[T]) Register(server *fsmserver.Server) error {
	if server == nil {
		return errors.New("fsm server is nil")
	}

	return server.Register(fsm)
}

// SetTransitions validates and atomically registers transition rules.
func (fsm *FSM[T]) SetTransitions(items ...TransitionItem[T]) *FSM[T] {
	fsm.configLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.configLock.Unlock()

	seen := make(map[EventStateKey]struct{}, len(items))
	for _, item := range items {
		key := EventStateKey{Event: item.Event, FromState: item.FromState}
		if _, exists := fsm.transitions[key]; exists {
			panic(ErrorEventStateKeyRepeated(key))
		}
		if _, repeated := seen[key]; repeated {
			panic(ErrorEventStateKeyRepeated(key))
		}
		seen[key] = struct{}{}
	}
	for _, item := range items {
		key := EventStateKey{Event: item.Event, FromState: item.FromState}
		fsm.transitions[key] = item.ToState
		fsm.handlers[key] = item.Handler
		fsm.setEventSearch(key)
	}

	return fsm
}

func (fsm *FSM[T]) setEventSearch(key EventStateKey) {
	if _, ok := fsm.eventSearch[key.FromState]; !ok {
		fsm.eventSearch[key.FromState] = []Event{}
	}
	fsm.eventSearch[key.FromState] = append(
		fsm.eventSearch[key.FromState], key.Event,
	)
}

// GetEventUnderState returns a copy of the events accepted by state.
func (fsm *FSM[T]) GetEventUnderState(state State) []Event {
	fsm.configLock.RLock()
	defer fsm.configLock.RUnlock()

	return append([]Event(nil), fsm.eventSearch[state]...)
}

func (fsm *FSM[T]) getTransition(key EventStateKey) (State, Handler[T], bool) {
	fsm.configLock.RLock()
	defer fsm.configLock.RUnlock()
	state, exists := fsm.transitions[key]

	return state, fsm.handlers[key], exists
}

// SetPayload replaces the FSM payload under the state lock.
func (fsm *FSM[T]) SetPayload(payload T) *FSM[T] {
	fsm.stateLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.stateLock.Unlock()
	fsm.Payload = payload

	return fsm
}

// SetCurrentState directly updates state without running a transition handler.
func (fsm *FSM[T]) SetCurrentState(state State) {
	fsm.stateLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.stateLock.Unlock()
	fsm.currentState = state
}

// GetCurrentState returns the current committed state.
func (fsm *FSM[T]) GetCurrentState() State {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	return fsm.currentState
}

// Metadata returns an independent snapshot of the FSM and serialized payload.
func (fsm *FSM[T]) Metadata() Metadata {
	if fsm == nil {
		return Metadata{Payload: json.RawMessage("null")}
	}
	fsm.stateLock.RLock()
	metadata := Metadata{
		InstanceID:      fsm.InstanceID,
		TransitionID:    fsm.TtId,
		Name:            fsm.Name,
		Description:     fsm.Describe,
		PreviousState:   fsm.fromState,
		PreviousEvent:   fsm.fromEvent,
		CurrentState:    fsm.currentState,
		ProcessingEvent: fsm.processingEvent,
		Payload:         json.RawMessage("null"),
	}
	payload := fsm.Payload
	fsm.stateLock.RUnlock()
	if isNilPayload(payload) {
		return metadata
	}
	payloadJSON, err := payload.JSON()
	if err != nil {
		metadata.PayloadError = err.Error()

		return metadata
	}
	if len(payloadJSON) == 0 {
		return metadata
	}
	if !json.Valid(payloadJSON) {
		metadata.PayloadError = "payload JSON is invalid"

		return metadata
	}
	metadata.Payload = append(json.RawMessage(nil), payloadJSON...)

	return metadata
}

func isNilPayload[T IPayload](payload T) bool {
	value := reflect.ValueOf(payload)
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// GetEventContext returns the cancellation context of the currently running event.
func (fsm *FSM[T]) GetEventContext() context.Context {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()
	if fsm.processingContext == nil {
		return context.Background()
	}

	return fsm.processingContext
}

func (fsm *FSM[T]) stateSnapshot() (State, string) {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	return fsm.currentState, fsm.TtId
}

func (fsm *FSM[T]) beginProcessingEvent(
	ctx context.Context,
	event Event,
) (State, string) {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()
	fsm.processingEvent, fsm.processingContext = event, ctx

	return fsm.currentState, fsm.TtId
}

func (fsm *FSM[T]) clearProcessingEvent() {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()
	fsm.processingEvent, fsm.processingContext = EventEmpty, context.Background()
}

func (fsm *FSM[T]) commitTransition(
	fromState State,
	event Event,
	toState State,
	ttID string,
) (State, error) {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()
	if fsm.currentState != fromState {
		return fsm.currentState, ErrorStateChanged
	}
	fsm.fromState, fsm.fromEvent, fsm.processingEvent = fromState, event, EventEmpty
	fsm.currentState, fsm.TtId = toState, ttID

	return fsm.currentState, nil
}
