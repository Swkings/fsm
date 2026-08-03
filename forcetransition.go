package fsm

import (
	"context"

	"github.com/google/uuid"
	"github.com/swkings/fsm/ds"
)

// PayloadUpdater updates the payload as part of a manual forced transition.
// Returning an error prevents the target state from being committed.
type PayloadUpdater[T IPayload] func(T) error

// ForceTransition manually moves the FSM to toState without requiring a
// configured event transition. Optional payload updaters run in order before
// the state is committed.
func (fsm *FSM[T]) ForceTransition(
	toState State,
	updatePayload ...PayloadUpdater[T],
) (TransitionTrace, error) {
	fsm.directEventPending.Add(1)
	fsm.eventScheduleLock.Lock()
	acquired := fsm.eventLock.TryLock()
	fsm.eventScheduleLock.Unlock()
	fsm.directEventPending.Add(-1)
	if !acquired {
		trace := fsm.rejectedManualTransitionTrace(
			toState, ErrorEventProcessing,
		)
		fsm.notifyRuntimeObservers(&trace)

		return trace, ErrorEventProcessing
	}
	defer fsm.releaseEventExecution()

	trace, err := fsm.forceTransitionLocked(toState, updatePayload...)
	persistErr := fsm.persistTrace(context.Background(), trace)
	fsm.notifyRuntimeObservers(&trace)
	if err == nil && persistErr != nil {
		return trace, persistErr
	}

	return trace, err
}

func (fsm *FSM[T]) forceTransitionLocked(
	toState State,
	updatePayload ...PayloadUpdater[T],
) (
	result TransitionTrace,
	resultErr error,
) {
	fromState, previousID := fsm.stateSnapshot()
	nextID := uuid.NewString()
	trace := ds.NewTransitionTrace(
		previousID, nextID, fromState, EventEmpty,
	).SetOperationBy(OperationByManual).Start()
	trace.SetToState(toState).SetEntryMetadata(fsm.Metadata())
	defer func() {
		trace.SetExitMetadata(fsm.Metadata())
		if resultErr != nil {
			trace.End(resultErr)
		} else {
			trace.End()
		}
		result = trace.TrimPtr()
	}()

	for _, update := range updatePayload {
		if update == nil {
			continue
		}
		if resultErr = update(fsm.Payload); resultErr != nil {
			trace.SetFinalState(fsm.GetCurrentState()).TransitionFail()

			return
		}
	}

	var finalState State
	finalState, resultErr = fsm.commitTransition(
		fromState, EventEmpty, toState, nextID,
	)
	if resultErr != nil {
		trace.SetFinalState(finalState).TransitionFail()

		return
	}
	trace.SetFinalState(finalState).TransitionSuccess()

	return
}

func (fsm *FSM[T]) rejectedManualTransitionTrace(
	toState State,
	err error,
) TransitionTrace {
	state, previousID := fsm.stateSnapshot()
	trace := ds.NewTransitionTrace(
		previousID, uuid.NewString(), state, EventEmpty,
	).SetOperationBy(OperationByManual).Start()
	trace.SetToState(toState).SetFinalState(state).TransitionFail().End(err)

	return trace.TrimPtr()
}
