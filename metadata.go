package fsm

import (
	"context"
	"encoding/json"
	"reflect"
)

// SetPayload replaces the FSM payload under the state lock.
func (fsm *FSM[T]) SetPayload(payload T) *FSM[T] {
	fsm.stateLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.stateLock.Unlock()
	fsm.Payload = payload

	return fsm
}

// SetCurrentState directly updates state without running a transition handler.
func (fsm *FSM[T]) SetCurrentState(state FSMState) {
	fsm.stateLock.Lock()
	defer fsm.notifyRuntimeObservers(nil)
	defer fsm.stateLock.Unlock()
	fsm.currentState = state
}

// GetCurrentState returns the current committed state.
func (fsm *FSM[T]) GetCurrentState() FSMState {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	return fsm.currentState
}

// Metadata returns an independent snapshot of the FSM and serialized payload.
func (fsm *FSM[T]) Metadata() FSMMetadata {
	if fsm == nil {
		return FSMMetadata{Payload: json.RawMessage("null")}
	}
	fsm.stateLock.RLock()
	metadata := FSMMetadata{
		InstanceID: fsm.InstanceID, TransitionID: fsm.TtId, Name: fsm.Name,
		Description: fsm.Describe, PreviousState: fsm.fromState, PreviousEvent: fsm.fromEvent,
		CurrentState: fsm.currentState, ProcessingEvent: fsm.processingEvent, Payload: json.RawMessage("null"),
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

func isNilPayload[T PayloadI](payload T) bool {
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

func (fsm *FSM[T]) stateSnapshot() (FSMState, string) {
	fsm.stateLock.RLock()
	defer fsm.stateLock.RUnlock()

	return fsm.currentState, fsm.TtId
}

func (fsm *FSM[T]) beginProcessingEvent(ctx context.Context, event FSMEvent) (FSMState, string) {
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

func (fsm *FSM[T]) commitTransition(fromState FSMState, event FSMEvent, toState FSMState, ttID string) (FSMState, error) {
	fsm.stateLock.Lock()
	defer fsm.stateLock.Unlock()
	if fsm.currentState != fromState {
		return fsm.currentState, ErrorStateChanged
	}
	fsm.fromState, fsm.fromEvent, fsm.processingEvent = fromState, event, EventEmpty
	fsm.currentState, fsm.TtId = toState, ttID

	return fsm.currentState, nil
}
