package fsm

import (
	"context"
	"sync"
	"time"
)

type queuedEvent struct {
	id          string
	event       FSMEvent
	priority    EventPriority
	ctx         context.Context
	notify      chan<- AsyncEventResult
	receipt     *AsyncEventReceipt
	submittedAt time.Time
	expiresAt   time.Time
	expiryTimer *time.Timer
}

type eventQueue struct {
	mu        sync.Mutex
	high, low []*queuedEvent
	wake      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	closed    bool
}

func newEventQueue() *eventQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &eventQueue{wake: make(chan struct{}, 1), ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

func (queue *eventQueue) enqueue(item *queuedEvent, onExpire func()) error {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return ErrorEventQueueClosed
	}
	if item.priority == EventPriorityHigh {
		queue.high = append(queue.high, item)
	} else {
		queue.low = append(queue.low, item)
	}
	if !item.expiresAt.IsZero() {
		item.expiryTimer = time.AfterFunc(time.Until(item.expiresAt), onExpire)
	}
	return nil
}

func (queue *eventQueue) dequeue() *queuedEvent {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.high) > 0 {
		item := queue.high[0]
		queue.high[0] = nil
		queue.high = queue.high[1:]
		return item
	}
	if len(queue.low) > 0 {
		item := queue.low[0]
		queue.low[0] = nil
		queue.low = queue.low[1:]
		return item
	}
	return nil
}

func (queue *eventQueue) remove(target *queuedEvent) bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	var removed bool
	queue.high, removed = removeQueuedEvent(queue.high, target)
	if removed {
		return true
	}
	queue.low, removed = removeQueuedEvent(queue.low, target)
	return removed
}

func removeQueuedEvent(items []*queuedEvent, target *queuedEvent) ([]*queuedEvent, bool) {
	for index, item := range items {
		if item != target {
			continue
		}
		copy(items[index:], items[index+1:])
		items[len(items)-1] = nil
		return items[:len(items)-1], true
	}
	return items, false
}

func (queue *eventQueue) requeueFront(item *queuedEvent) bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return false
	}
	if item.priority == EventPriorityHigh {
		queue.high = append(queue.high, nil)
		copy(queue.high[1:], queue.high)
		queue.high[0] = item
	} else {
		queue.low = append(queue.low, nil)
		copy(queue.low[1:], queue.low)
		queue.low[0] = item
	}
	return true
}

func (queue *eventQueue) hasHighPriorityEvent() bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return len(queue.high) > 0
}

func (queue *eventQueue) close() (pending []*queuedEvent, alreadyClosed bool) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return nil, true
	}
	queue.closed = true
	pending = append(pending, queue.high...)
	pending = append(pending, queue.low...)
	queue.high, queue.low = nil, nil
	queue.cancel()
	return pending, false
}

func (queue *eventQueue) signal() {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}
