package fsm

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

func (fsm *FSM[T]) runEventQueue() {
	defer close(fsm.eventQueue.done)
	for {
		if err := fsm.eventQueue.ctx.Err(); err != nil {
			return
		}
		item := fsm.eventQueue.dequeue()
		if item == nil {
			select {
			case <-fsm.eventQueue.wake:
				continue
			case <-fsm.eventQueue.ctx.Done():
				return
			}
		}
		if status, err, canceled := queuedEventCancellation(item); canceled {
			fsm.rejectQueuedEvent(item, status, err)

			continue
		}
		if item.priority == EventPriorityLow && fsm.eventQueue.hasHighPriorityEvent() {
			if !fsm.eventQueue.requeueFront(item) {
				fsm.rejectQueuedEvent(item, AsyncEventCanceled, ErrorEventQueueClosed)
			}

			continue
		}
		if !fsm.tryAcquireQueuedEvent() {
			if !fsm.eventQueue.requeueFront(item) {
				fsm.rejectQueuedEvent(item, AsyncEventCanceled, ErrorEventQueueClosed)

				continue
			}
			fsm.waitForQueueSchedule()

			continue
		}
		stopEventExpiryTimer(item)
		startedAt := time.Now()
		executionContext, cancel := eventExecutionContext(item.ctx, fsm.eventQueue.ctx)
		trace, err := fsm.processEventLocked(executionContext, item.event)
		cancel()
		fsm.releaseEventExecution()
		status := AsyncEventSucceeded
		if err != nil {
			status = AsyncEventFailed
			if item.ctx != nil && (item.ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				status = AsyncEventCanceled
			}
		}
		fsm.completeQueuedEvent(item, status, trace, err, startedAt)
	}
}

func (fsm *FSM[T]) tryAcquireQueuedEvent() bool {
	fsm.eventScheduleLock.Lock()
	defer fsm.eventScheduleLock.Unlock()
	if fsm.directEventPending.Load() > 0 {
		return false
	}

	return fsm.eventLock.TryLock()
}

func (fsm *FSM[T]) waitForQueueSchedule() {
	select {
	case <-fsm.eventQueue.wake:
	case <-fsm.eventQueue.ctx.Done():
	}
}

func queuedEventCancellation(item *queuedEvent) (AsyncEventStatus, error, bool) {
	if !item.expiresAt.IsZero() && !time.Now().Before(item.expiresAt) {
		return AsyncEventExpired, context.DeadlineExceeded, true
	}
	if item.ctx == nil {
		return "", nil, false
	}
	select {
	case <-item.ctx.Done():
		return AsyncEventCanceled, item.ctx.Err(), true
	default:
		return "", nil, false
	}
}

func eventExecutionContext(parent, queueContext context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		return context.Background(), func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	stopQueueCancel := context.AfterFunc(queueContext, cancel)
	if queueContext.Err() != nil {
		cancel()
	}

	return ctx, func() { stopQueueCancel(); cancel() }
}

func stopEventExpiryTimer(item *queuedEvent) {
	if item.expiryTimer != nil {
		item.expiryTimer.Stop()
	}
}

func (fsm *FSM[T]) rejectQueuedEvent(item *queuedEvent, status AsyncEventStatus, err error) {
	stopEventExpiryTimer(item)
	if item.receipt == nil && !fsm.hasRuntimeObservers() {
		return
	}
	trace := fsm.rejectedEventTrace(item.event, err)
	fsm.notifyRuntimeObservers(&trace)
	if item.receipt == nil {
		return
	}
	fsm.completeQueuedEvent(item, status, trace, err, time.Time{})
}

func (fsm *FSM[T]) rejectedEventTrace(event FSMEvent, err error) TransitionTrace {
	state, previousID := fsm.stateSnapshot()
	trace := NewTransitionTrace(previousID, uuid.NewString(), state, event).Start()
	trace.SetFinalState(state).TransitionFail().End(err)

	return trace.TrimPtr()
}

func (fsm *FSM[T]) completeQueuedEvent(item *queuedEvent, status AsyncEventStatus, trace TransitionTrace, err error, startedAt time.Time) {
	stopEventExpiryTimer(item)
	if item.receipt == nil {
		return
	}
	result := AsyncEventResult{
		ID: item.id, Event: item.event, Priority: item.priority, Status: status,
		Trace: trace, Error: err, SubmittedAt: item.submittedAt, StartedAt: startedAt, FinishedAt: time.Now(),
	}
	if !item.receipt.complete(result) {
		return
	}
	go func() { defer func() { _ = recover() }(); item.notify <- result }()
}
