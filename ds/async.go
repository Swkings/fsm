package ds

import (
	"time"

	commonasync "github.com/swkings/fsm/common/async"
)

// EventPriority controls asynchronous queue scheduling.
type EventPriority = commonasync.Priority

const (
	EventPriorityLow  = commonasync.PriorityLow
	EventPriorityHigh = commonasync.PriorityHigh
)

// AsyncEventStatus is the terminal state of an asynchronous event.
type AsyncEventStatus string

const (
	AsyncEventSucceeded AsyncEventStatus = "succeeded"
	AsyncEventFailed    AsyncEventStatus = "failed"
	AsyncEventCanceled  AsyncEventStatus = "canceled"
	AsyncEventExpired   AsyncEventStatus = "expired"
)

// AsyncEventResult is the immutable result delivered for a tracked event.
type AsyncEventResult struct {
	ID          string
	Event       FSMEvent
	Priority    EventPriority
	Status      AsyncEventStatus
	Trace       TransitionTrace
	Error       error
	SubmittedAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
}
