package fsm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	commonds "github.com/swkings/fsm/ds"
)

type (
	EventPriority    = commonds.EventPriority
	AsyncEventStatus = commonds.AsyncEventStatus
	AsyncEventResult = commonds.AsyncEventResult
)

const (
	EventPriorityLow    = commonds.EventPriorityLow
	EventPriorityHigh   = commonds.EventPriorityHigh
	AsyncEventSucceeded = commonds.AsyncEventSucceeded
	AsyncEventFailed    = commonds.AsyncEventFailed
	AsyncEventCanceled  = commonds.AsyncEventCanceled
	AsyncEventExpired   = commonds.AsyncEventExpired
)

var (
	ErrorEventQueueClosed           = errors.New("fsm event queue is closed")
	ErrorInvalidEventPriority       = errors.New("invalid async event priority")
	ErrorInvalidEventTTL            = errors.New("invalid async event ttl")
	ErrorNilEventContext            = errors.New("async event context is nil")
	ErrorNilEventNotify             = errors.New("async event notify channel is nil")
	ErrorEventContextNotifyMismatch = errors.New("async event context and notify must be set together")
)

// AsyncEventReceipt provides completion notification and result lookup.
type AsyncEventReceipt struct {
	ID        string
	done      chan struct{}
	once      sync.Once
	resultMu  sync.RWMutex
	result    AsyncEventResult
	completed bool
}

func newAsyncEventReceipt(id string) *AsyncEventReceipt {
	return &AsyncEventReceipt{ID: id, done: make(chan struct{})}
}

// Done closes when the event reaches a terminal state.
func (receipt *AsyncEventReceipt) Done() <-chan struct{} { return receipt.done }

// Result returns the terminal result, or ok=false while the event is pending.
func (receipt *AsyncEventReceipt) Result() (result AsyncEventResult, ok bool) {
	receipt.resultMu.RLock()
	defer receipt.resultMu.RUnlock()

	return receipt.result, receipt.completed
}

// Wait waits for completion without changing the submitted event's context.
func (receipt *AsyncEventReceipt) Wait(ctx context.Context) (AsyncEventResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-receipt.done:
		result, _ := receipt.Result()

		return result, nil
	default:
	}
	select {
	case <-receipt.done:
		result, _ := receipt.Result()

		return result, nil
	case <-ctx.Done():
		select {
		case <-receipt.done:
			result, _ := receipt.Result()

			return result, nil
		default:
		}

		return AsyncEventResult{}, ctx.Err()
	}
}

func (receipt *AsyncEventReceipt) complete(result AsyncEventResult) (completed bool) {
	receipt.once.Do(func() {
		receipt.resultMu.Lock()
		receipt.result, receipt.completed = result, true
		receipt.resultMu.Unlock()
		close(receipt.done)
		completed = true
	})

	return completed
}

type asyncEventConfig struct {
	priority  EventPriority
	ttl       time.Duration
	expiresAt time.Time
	ctx       context.Context
	notify    chan<- AsyncEventResult
}

// AsyncEventOption configures one asynchronous submission.
type AsyncEventOption func(*asyncEventConfig) error

// WithEventPriority configures queue priority.
func WithEventPriority(priority EventPriority) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if priority != EventPriorityLow && priority != EventPriorityHigh {
			return fmt.Errorf("%w: %d", ErrorInvalidEventPriority, priority)
		}
		config.priority = priority

		return nil
	}
}

// WithEventTTL configures how long an event may wait in the queue.
func WithEventTTL(ttl time.Duration) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if ttl < 0 {
			return fmt.Errorf("%w: %s", ErrorInvalidEventTTL, ttl)
		}
		config.ttl, config.expiresAt = ttl, time.Time{}

		return nil
	}
}

// WithEventExpiresAt configures an absolute queue expiration time.
func WithEventExpiresAt(expiresAt time.Time) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		config.expiresAt, config.ttl = expiresAt, 0

		return nil
	}
}

// WithEventContext configures caller cancellation and must be paired with WithEventNotify.
func WithEventContext(ctx context.Context) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if ctx == nil {
			return ErrorNilEventContext
		}
		config.ctx = ctx

		return nil
	}
}

// WithEventNotify configures result delivery and must be paired with WithEventContext.
func WithEventNotify(notify chan<- AsyncEventResult) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if notify == nil {
			return ErrorNilEventNotify
		}
		config.notify = notify

		return nil
	}
}

// SubmitEvent queues an event. Omitting context and notify uses fire-and-forget mode.
func (fsm *FSM[T]) SubmitEvent(event Event, options ...AsyncEventOption) (*AsyncEventReceipt, error) {
	config := asyncEventConfig{priority: EventPriorityHigh}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&config); err != nil {
			return nil, err
		}
	}
	if (config.ctx == nil) != (config.notify == nil) {
		return nil, ErrorEventContextNotifyMismatch
	}
	submittedAt := time.Now()
	id := ""
	if config.ctx != nil {
		id = uuid.NewString()
	}
	item := &queuedEvent{
		id: id, event: event, priority: config.priority, ctx: config.ctx,
		notify: config.notify, submittedAt: submittedAt,
	}
	if config.ctx != nil {
		item.receipt = newAsyncEventReceipt(id)
	}
	if !config.expiresAt.IsZero() {
		item.expiresAt = config.expiresAt
	} else if config.ttl > 0 {
		item.expiresAt = submittedAt.Add(config.ttl)
	}
	if err := fsm.eventQueue.enqueue(item, func() {
		if fsm.eventQueue.remove(item) {
			fsm.rejectQueuedEvent(item, AsyncEventExpired, context.DeadlineExceeded)

			return
		}
		fsm.eventQueue.signal()
	}); err != nil {
		return nil, err
	}
	fsm.eventQueue.startOnce.Do(func() { go fsm.runEventQueue() })
	if item.ctx != nil {
		go func() {
			select {
			case <-item.ctx.Done():
				if fsm.eventQueue.remove(item) {
					fsm.rejectQueuedEvent(item, AsyncEventCanceled, item.ctx.Err())

					return
				}
				fsm.eventQueue.signal()
			case <-item.receipt.Done():
			}
		}()
	}
	fsm.eventQueue.signal()

	return item.receipt, nil
}

// PendingEventCount returns the number of queued high- and low-priority events.
func (fsm *FSM[T]) PendingEventCount() (high, low int) {
	fsm.eventQueue.mu.Lock()
	defer fsm.eventQueue.mu.Unlock()

	return len(fsm.eventQueue.high), len(fsm.eventQueue.low)
}

// Close cancels queued tracked events and waits for the asynchronous worker.
func (fsm *FSM[T]) Close() error {
	pending, alreadyClosed := fsm.eventQueue.close()
	if !alreadyClosed {
		for _, item := range pending {
			fsm.rejectQueuedEvent(item, AsyncEventCanceled, ErrorEventQueueClosed)
		}
		fsm.eventQueue.startOnce.Do(func() { go fsm.runEventQueue() })
		fsm.eventQueue.signal()
	}
	<-fsm.eventQueue.done

	return nil
}
