package ds

import "time"

// EventPriority controls asynchronous queue scheduling.
type EventPriority uint8

const (
	EventPriorityLow EventPriority = iota
	EventPriorityHigh
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
