package fsm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventPriority 表示异步事件在队列中的调度优先级。
type EventPriority uint8

const (
	// EventPriorityLow 仅在没有高优先级事件且没有同步事件正在竞争执行权时运行。
	EventPriorityLow EventPriority = iota
	// EventPriorityHigh 优先于低优先级事件，但低于直接调用 ProcessEvent 的同步事件。
	EventPriorityHigh
)

// AsyncEventStatus 表示异步事件的最终状态。
type AsyncEventStatus string

const (
	// AsyncEventSucceeded 表示事件处理成功，目标状态已经提交。
	AsyncEventSucceeded AsyncEventStatus = "succeeded"
	// AsyncEventFailed 表示迁移规则不存在或 Handler 返回错误。
	AsyncEventFailed AsyncEventStatus = "failed"
	// AsyncEventCanceled 表示提交方 context 或状态机队列被取消。
	AsyncEventCanceled AsyncEventStatus = "canceled"
	// AsyncEventExpired 表示事件在开始前或处理期间超过了有效期。
	AsyncEventExpired AsyncEventStatus = "expired"
)

var (
	// ErrorEventQueueClosed 表示状态机异步事件队列已经关闭。
	ErrorEventQueueClosed = errors.New("fsm event queue is closed")
	// ErrorInvalidEventPriority 表示异步事件优先级无效。
	ErrorInvalidEventPriority = errors.New("invalid async event priority")
	// ErrorInvalidEventTTL 表示异步事件过期时间为负数。
	ErrorInvalidEventTTL = errors.New("invalid async event ttl")
	// ErrorNilEventContext 表示传入了 nil 事件 context。
	ErrorNilEventContext = errors.New("async event context is nil")
	// ErrorNilEventNotify 表示传入了 nil 事件通知通道。
	ErrorNilEventNotify = errors.New("async event notify channel is nil")
)

// AsyncEventResult 是异步事件完成、失败、取消或过期后的最终结果。
type AsyncEventResult struct {
	// ID 是 SubmitEvent 为本次提交生成的唯一标识。
	ID string
	// Event 和 Priority 分别记录事件名称及其队列优先级。
	Event    FSMEvent
	Priority EventPriority
	// Status 是事件唯一的最终状态。
	Status AsyncEventStatus
	// Trace 和 Error 分别保存迁移追踪及最终错误；成功时 Error 为 nil。
	Trace TransitionTrace
	Error error
	// SubmittedAt、StartedAt 和 FinishedAt 可用于计算排队及执行耗时。
	// 未进入 Handler 的取消或过期事件，其 StartedAt 为零值。
	SubmittedAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
}

// AsyncEventReceipt 为提交方提供不会丢失的完成通知和结果查询能力。
type AsyncEventReceipt struct {
	// ID 与最终 AsyncEventResult.ID 相同，可用于关联日志和外部任务。
	ID string

	done      chan struct{}
	once      sync.Once
	resultMu  sync.RWMutex
	result    AsyncEventResult
	completed bool
}

func newAsyncEventReceipt(id string) *AsyncEventReceipt {
	return &AsyncEventReceipt{ID: id, done: make(chan struct{})}
}

// Done 在异步事件进入任一最终状态时关闭。
func (receipt *AsyncEventReceipt) Done() <-chan struct{} {
	return receipt.done
}

// Result 返回最终结果；事件尚未结束时 ok 为 false。
func (receipt *AsyncEventReceipt) Result() (result AsyncEventResult, ok bool) {
	receipt.resultMu.RLock()
	defer receipt.resultMu.RUnlock()
	return receipt.result, receipt.completed
}

// Wait 等待异步事件结束，或等待调用方提供的 context 取消。
// Wait 的 context 只影响本次等待，不会取消已经提交的事件。
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

func (receipt *AsyncEventReceipt) complete(result AsyncEventResult) {
	receipt.once.Do(func() {
		receipt.resultMu.Lock()
		receipt.result = result
		receipt.completed = true
		receipt.resultMu.Unlock()
		close(receipt.done)
	})
}

type asyncEventConfig struct {
	priority  EventPriority
	ttl       time.Duration
	expiresAt time.Time
	ctx       context.Context
	notify    chan<- AsyncEventResult
}

// AsyncEventOption 配置异步事件的优先级、过期时间、取消上下文和额外通知通道。
type AsyncEventOption func(*asyncEventConfig) error

// WithEventPriority 设置异步事件优先级。默认值为 EventPriorityHigh。
func WithEventPriority(priority EventPriority) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if priority != EventPriorityLow && priority != EventPriorityHigh {
			return fmt.Errorf("%w: %d", ErrorInvalidEventPriority, priority)
		}
		config.priority = priority
		return nil
	}
}

// WithEventTTL 设置事件从提交时开始计算的存活时间。零值表示永不过期，负数无效。
func WithEventTTL(ttl time.Duration) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if ttl < 0 {
			return fmt.Errorf("%w: %s", ErrorInvalidEventTTL, ttl)
		}
		config.ttl = ttl
		config.expiresAt = time.Time{}
		return nil
	}
}

// WithEventExpiresAt 设置事件的绝对过期时间。零值表示永不过期。
func WithEventExpiresAt(expiresAt time.Time) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		config.expiresAt = expiresAt
		config.ttl = 0
		return nil
	}
}

// WithEventContext 设置事件取消上下文。context 取消会取消排队中或处理中事件。
func WithEventContext(ctx context.Context) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if ctx == nil {
			return ErrorNilEventContext
		}
		config.ctx = ctx
		return nil
	}
}

// WithEventNotify 设置额外结果通知通道。
// 队列不会关闭调用方拥有的通道；通知发送在独立 goroutine 中执行，不阻塞后续事件。
// 调用方应持续接收该通道，推荐使用至少容量为 1 的缓冲通道，并且不要在收到结果前关闭通道。
func WithEventNotify(notify chan<- AsyncEventResult) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if notify == nil {
			return ErrorNilEventNotify
		}
		config.notify = notify
		return nil
	}
}

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
	high      []*queuedEvent
	low       []*queuedEvent
	wake      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	closed    bool
}

func newEventQueue() *eventQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &eventQueue{
		wake:   make(chan struct{}, 1),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

// SubmitEvent 将事件提交到异步优先级队列。
// 默认使用高优先级、context.Background，且永不过期。
func (fsm *FSM[T]) SubmitEvent(event FSMEvent, options ...AsyncEventOption) (*AsyncEventReceipt, error) {
	config := asyncEventConfig{priority: EventPriorityHigh, ctx: context.Background()}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&config); err != nil {
			return nil, err
		}
	}

	submittedAt := time.Now()
	id := uuid.NewString()
	item := &queuedEvent{
		id:          id,
		event:       event,
		priority:    config.priority,
		ctx:         config.ctx,
		notify:      config.notify,
		receipt:     newAsyncEventReceipt(id),
		submittedAt: submittedAt,
	}
	if !config.expiresAt.IsZero() {
		item.expiresAt = config.expiresAt
	} else if config.ttl > 0 {
		item.expiresAt = submittedAt.Add(config.ttl)
	}

	if !item.expiresAt.IsZero() {
		item.expiryTimer = time.AfterFunc(time.Until(item.expiresAt), fsm.eventQueue.signal)
	}
	if err := fsm.eventQueue.enqueue(item); err != nil {
		if item.expiryTimer != nil {
			item.expiryTimer.Stop()
		}
		return nil, err
	}
	fsm.eventQueue.startOnce.Do(func() { go fsm.runEventQueue() })
	go func() {
		select {
		case <-item.ctx.Done():
			fsm.eventQueue.signal()
		case <-item.receipt.Done():
		}
	}()
	fsm.eventQueue.signal()
	return item.receipt, nil
}

// PendingEventCount 返回仍位于高、低优先级队列中的事件数量。
func (fsm *FSM[T]) PendingEventCount() (high int, low int) {
	fsm.eventQueue.mu.Lock()
	defer fsm.eventQueue.mu.Unlock()
	return len(fsm.eventQueue.high), len(fsm.eventQueue.low)
}

// Close 取消队列中和正在执行的异步事件，并等待后台 worker 退出。
// Close 不影响之后直接调用 ProcessEvent，但关闭后不能再次 SubmitEvent。
// 不要从异步事件自己的 Handler 内调用 Close，否则会等待当前 Handler 自身退出。
func (fsm *FSM[T]) Close() error {
	pending, alreadyClosed := fsm.eventQueue.close()
	if !alreadyClosed {
		for _, item := range pending {
			fsm.completeQueuedEvent(item, AsyncEventCanceled, fsm.rejectedEventTrace(item.event, ErrorEventQueueClosed), ErrorEventQueueClosed, time.Time{})
		}
		fsm.eventQueue.startOnce.Do(func() { go fsm.runEventQueue() })
		fsm.eventQueue.signal()
	}
	<-fsm.eventQueue.done
	return nil
}

func (queue *eventQueue) enqueue(item *queuedEvent) error {
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
	queue.high = nil
	queue.low = nil
	queue.cancel()
	return pending, false
}

func (queue *eventQueue) signal() {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}

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
			trace := fsm.rejectedEventTrace(item.event, err)
			fsm.completeQueuedEvent(item, status, trace, err, time.Time{})
			continue
		}
		if item.priority == EventPriorityLow && fsm.eventQueue.hasHighPriorityEvent() {
			if !fsm.eventQueue.requeueFront(item) {
				fsm.completeQueuedEvent(item, AsyncEventCanceled, fsm.rejectedEventTrace(item.event, ErrorEventQueueClosed), ErrorEventQueueClosed, time.Time{})
				continue
			}
			continue
		}
		if !fsm.tryAcquireQueuedEvent() {
			if !fsm.eventQueue.requeueFront(item) {
				fsm.completeQueuedEvent(item, AsyncEventCanceled, fsm.rejectedEventTrace(item.event, ErrorEventQueueClosed), ErrorEventQueueClosed, time.Time{})
				continue
			}
			fsm.waitForQueueSchedule()
			continue
		}

		startedAt := time.Now()
		executionContext, cancel := eventExecutionContext(item.ctx, fsm.eventQueue.ctx, item.expiresAt)
		trace, err := fsm.processEventLocked(executionContext, item.event)
		cancel()
		fsm.releaseEventExecution()

		status := AsyncEventSucceeded
		if err != nil {
			status = AsyncEventFailed
			if !item.expiresAt.IsZero() && !time.Now().Before(item.expiresAt) {
				status = AsyncEventExpired
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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
	select {
	case <-item.ctx.Done():
		return AsyncEventCanceled, item.ctx.Err(), true
	default:
		return "", nil, false
	}
}

func eventExecutionContext(parent context.Context, queueContext context.Context, expiresAt time.Time) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if expiresAt.IsZero() {
		ctx, cancel = context.WithCancel(parent)
	} else {
		ctx, cancel = context.WithDeadline(parent, expiresAt)
	}
	stopQueueCancel := context.AfterFunc(queueContext, cancel)
	if queueContext.Err() != nil {
		cancel()
	}
	return ctx, func() {
		stopQueueCancel()
		cancel()
	}
}

func (fsm *FSM[T]) rejectedEventTrace(event FSMEvent, err error) TransitionTrace {
	state, previousID := fsm.stateSnapshot()
	trace := NewTransitionTrace(previousID, uuid.NewString(), state, event).Start()
	trace.SetFinalState(state).TransitionFail().End(err)
	return trace.TrimPtr()
}

func (fsm *FSM[T]) completeQueuedEvent(item *queuedEvent, status AsyncEventStatus, trace TransitionTrace, err error, startedAt time.Time) {
	if item.expiryTimer != nil {
		item.expiryTimer.Stop()
	}
	result := AsyncEventResult{
		ID:          item.id,
		Event:       item.event,
		Priority:    item.priority,
		Status:      status,
		Trace:       trace,
		Error:       err,
		SubmittedAt: item.submittedAt,
		StartedAt:   startedAt,
		FinishedAt:  time.Now(),
	}
	item.receipt.complete(result)
	if item.notify != nil {
		go func() {
			defer func() { _ = recover() }()
			item.notify <- result
		}()
	}
}
