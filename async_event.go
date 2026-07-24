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
	// AsyncEventFailed 表示转移规则不存在或 Handler 返回错误。
	AsyncEventFailed AsyncEventStatus = "failed"
	// AsyncEventCanceled 表示提交方 context 或状态机队列被取消。
	AsyncEventCanceled AsyncEventStatus = "canceled"
	// AsyncEventExpired 表示事件在队列中等待时超过了有效期。
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
	// ErrorEventContextNotifyMismatch 表示 context 和通知通道没有同时设置。
	ErrorEventContextNotifyMismatch = errors.New("async event context and notify must be set together")
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
	// Trace 和 Error 分别保存转移追踪及最终错误；成功时 Error 为 nil。
	Trace TransitionTrace
	Error error
	// SubmittedAt、StartedAt 和 FinishedAt 可用于计算排队及执行耗时。
	// 未进入 Handler 的取消或过期事件，其 StartedAt 为零值。
	SubmittedAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
}

// AsyncEventReceipt 为关心执行结果的提交方提供完成通知和结果查询能力。
// 只有同时设置 WithEventContext 和 WithEventNotify 时，SubmitEvent 才会返回非 nil receipt。
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

func (receipt *AsyncEventReceipt) complete(result AsyncEventResult) (completed bool) {
	receipt.once.Do(func() {
		receipt.resultMu.Lock()
		receipt.result = result
		receipt.completed = true
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

// AsyncEventOption 配置异步事件的优先级、排队有效期、取消上下文和通知通道。
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

// WithEventTTL 设置事件从提交时开始计算的排队有效期。
// 零值表示永不过期，负数无效；事件开始执行后不再受该有效期影响。
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

// WithEventExpiresAt 设置事件的绝对排队过期时间。零值表示永不过期。
// 事件开始执行后即使到达该时间，也不会因过期被取消。
func WithEventExpiresAt(expiresAt time.Time) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		config.expiresAt = expiresAt
		config.ttl = 0
		return nil
	}
}

// WithEventContext 设置提交方用于取消事件的 context。
// 该选项必须和 WithEventNotify 同时设置；Handler 可通过 GetEventContext 监听取消信号。
func WithEventContext(ctx context.Context) AsyncEventOption {
	return func(config *asyncEventConfig) error {
		if ctx == nil {
			return ErrorNilEventContext
		}
		config.ctx = ctx
		return nil
	}
}

// WithEventNotify 设置 FSM 向提交方发送最终事件状态的通知通道。
// 该选项必须和 WithEventContext 同时设置。FSM 不会关闭调用方拥有的通道；
// 通知发送在独立 goroutine 中执行，推荐使用至少容量为 1 的缓冲通道。
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
// 默认使用高优先级且永不过期。ctx 和 notify 必须同时设置或同时省略：
// 同时省略时采用 fire-and-forget 模式并返回 nil receipt；同时设置时可取消事件并接收最终状态。
func (fsm *FSM[T]) SubmitEvent(event FSMEvent, options ...AsyncEventOption) (*AsyncEventReceipt, error) {
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
		id:          id,
		event:       event,
		priority:    config.priority,
		ctx:         config.ctx,
		notify:      config.notify,
		submittedAt: submittedAt,
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

// PendingEventCount 返回仍位于高、低优先级队列中的事件数量。
func (fsm *FSM[T]) PendingEventCount() (high int, low int) {
	fsm.eventQueue.mu.Lock()
	defer fsm.eventQueue.mu.Unlock()
	return len(fsm.eventQueue.high), len(fsm.eventQueue.low)
}

// Close 取消队列中和正在执行的可通知事件，并等待后台 worker 退出。
// fire-and-forget 事件没有提交方 context，已开始执行后会自然运行到 Handler 返回。
// Close 不影响之后直接调用 ProcessEvent，但关闭后不能再次 SubmitEvent。
// 不要从异步事件自己的 Handler 内调用 Close，否则会等待当前 Handler 自身退出。
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

// enqueue 在持锁期间入队并安装过期计时器，避免过期回调早于入队执行。
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

// remove 仅移除仍位于队列切片中的事件；已经被 worker 取出的事件不受排队过期逻辑影响。
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
			fsm.rejectQueuedEvent(item, status, err)
			continue
		}
		if item.priority == EventPriorityLow && fsm.eventQueue.hasHighPriorityEvent() {
			if !fsm.eventQueue.requeueFront(item) {
				fsm.rejectQueuedEvent(item, AsyncEventCanceled, ErrorEventQueueClosed)
				continue
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

// eventExecutionContext 只合并提交方取消信号和队列关闭信号，不包含事件排队过期时间。
func eventExecutionContext(parent context.Context, queueContext context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		return context.Background(), func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	stopQueueCancel := context.AfterFunc(queueContext, cancel)
	if queueContext.Err() != nil {
		cancel()
	}
	return ctx, func() {
		stopQueueCancel()
		cancel()
	}
}

func stopEventExpiryTimer(item *queuedEvent) {
	if item.expiryTimer != nil {
		item.expiryTimer.Stop()
	}
}

// rejectQueuedEvent 只为设置了 ctx 和 notify 的事件构建结果；fire-and-forget 事件直接结束。
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
	if !item.receipt.complete(result) {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		item.notify <- result
	}()
}
