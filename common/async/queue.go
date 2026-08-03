// Package async provides concurrency-safe asynchronous queue primitives.
package async

import (
	"context"
	"errors"
	"sync"
)

// ErrQueueClosed is returned when an item is enqueued after the queue closes.
var ErrQueueClosed = errors.New("async queue is closed")

// Priority controls the order in which queued items are returned.
type Priority uint8

const (
	PriorityLow Priority = iota
	PriorityHigh
)

// Queue is a concurrency-safe two-priority asynchronous queue.
//
// Queue owns the cancellation, wake-up, and worker lifecycle signals used by
// consumers, while the consumer remains responsible for processing items.
type Queue[T comparable] struct {
	mu        sync.Mutex
	high      []T
	low       []T
	wake      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	closed    bool
}

// NewQueue creates an open queue.
func NewQueue[T comparable]() *Queue[T] {
	ctx, cancel := context.WithCancel(context.Background())

	return &Queue[T]{
		wake:   make(chan struct{}, 1),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

// Enqueue adds an item at the back of its priority group.
func (queue *Queue[T]) Enqueue(item T, priority Priority) error {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return ErrQueueClosed
	}
	if priority == PriorityHigh {
		queue.high = append(queue.high, item)
	} else {
		queue.low = append(queue.low, item)
	}

	return nil
}

// Dequeue returns the next high-priority item, falling back to low priority.
func (queue *Queue[T]) Dequeue() (item T, ok bool) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.high) > 0 {
		return popFront(&queue.high), true
	}
	if len(queue.low) > 0 {
		return popFront(&queue.low), true
	}

	return item, false
}

func popFront[T comparable](items *[]T) T {
	item := (*items)[0]
	var zero T
	(*items)[0] = zero
	*items = (*items)[1:]

	return item
}

// Remove removes the first queued occurrence of target.
func (queue *Queue[T]) Remove(target T) bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	var removed bool
	queue.high, removed = remove(queue.high, target)
	if removed {
		return true
	}
	queue.low, removed = remove(queue.low, target)

	return removed
}

func remove[T comparable](items []T, target T) ([]T, bool) {
	for index, item := range items {
		if item != target {
			continue
		}
		copy(items[index:], items[index+1:])
		var zero T
		items[len(items)-1] = zero

		return items[:len(items)-1], true
	}

	return items, false
}

// RequeueFront places an item at the front of its priority group.
// It returns false when the queue is already closed.
func (queue *Queue[T]) RequeueFront(item T, priority Priority) bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return false
	}
	if priority == PriorityHigh {
		queue.high = prepend(queue.high, item)
	} else {
		queue.low = prepend(queue.low, item)
	}

	return true
}

func prepend[T comparable](items []T, item T) []T {
	var zero T
	items = append(items, zero)
	copy(items[1:], items)
	items[0] = item

	return items
}

// HasPriority reports whether the priority group contains an item.
func (queue *Queue[T]) HasPriority(priority Priority) bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if priority == PriorityHigh {
		return len(queue.high) > 0
	}

	return len(queue.low) > 0
}

// Pending returns the number of high- and low-priority queued items.
func (queue *Queue[T]) Pending() (high, low int) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	return len(queue.high), len(queue.low)
}

// Close closes the queue, cancels its context, and returns all pending items.
// Pending items are ordered by priority, with high-priority items first.
func (queue *Queue[T]) Close() (pending []T, alreadyClosed bool) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return nil, true
	}
	queue.closed = true
	pending = make([]T, 0, len(queue.high)+len(queue.low))
	pending = append(pending, queue.high...)
	pending = append(pending, queue.low...)
	queue.high, queue.low = nil, nil
	queue.cancel()

	return pending, false
}

// Start launches worker at most once and closes Done when it returns.
func (queue *Queue[T]) Start(worker func()) {
	if worker == nil {
		return
	}
	queue.startOnce.Do(func() {
		go func() {
			defer close(queue.done)
			worker()
		}()
	})
}

// Context is canceled when the queue closes.
func (queue *Queue[T]) Context() context.Context { return queue.ctx }

// Wake receives a notification after Signal is called.
func (queue *Queue[T]) Wake() <-chan struct{} { return queue.wake }

// Done closes after the queue worker returns.
func (queue *Queue[T]) Done() <-chan struct{} { return queue.done }

// Signal wakes a waiting consumer. Multiple pending signals are coalesced.
func (queue *Queue[T]) Signal() {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}
