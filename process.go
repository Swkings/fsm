package fsm

import (
	"context"

	"github.com/google/uuid"
)

// ProcessEvent processes an event with a background context.
func (fsm *FSM[T]) ProcessEvent(event FSMEvent, params ...any) (TransitionTrace, error) {
	return fsm.ProcessEventWithContext(context.Background(), event, params...)
}

// ProcessEventWithContext processes an event and propagates cancellation between handler stages.
func (fsm *FSM[T]) ProcessEventWithContext(ctx context.Context, event FSMEvent, params ...any) (TransitionTrace, error) {
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
		return fsm.rejectedRuntimeEventTrace(event, ErrorEventProcessing), ErrorEventProcessing
	}
	defer fsm.releaseEventExecution()
	return fsm.processEventLocked(ctx, event, params...)
}

func (fsm *FSM[T]) processEventLocked(ctx context.Context, event FSMEvent, params ...any) (TransitionTrace, error) {
	trace, err := fsm.processEventLockedCore(ctx, event, params...)
	persistErr := fsm.persistTrace(context.Background(), trace)
	fsm.notifyRuntimeObservers(&trace)
	if err == nil && persistErr != nil {
		return trace, persistErr
	}
	return trace, err
}

func (fsm *FSM[T]) processEventLockedCore(ctx context.Context, event FSMEvent, params ...any) (result TransitionTrace, resultErr error) {
	if err := ctx.Err(); err != nil {
		return fsm.rejectedEventTrace(event, err), err
	}
	fromState, previousID := fsm.beginProcessingEvent(ctx, event)
	fsm.notifyRuntimeObservers(nil)
	nextID := uuid.NewString()
	trace := NewTransitionTrace(previousID, nextID, fromState, event).Start()
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
	var finalState FSMState
	finalState, resultErr = fsm.commitTransition(fromState, event, toState, nextID)
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
		fsm.eventQueue.signal()
	}
}
