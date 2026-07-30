package fsm

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAsyncEventPriorityOrder(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	defer func() { _ = machine.Close() }()

	blockStarted := make(chan struct{})
	releaseBlock := make(chan struct{})
	var orderMu sync.Mutex
	var order []string
	appendOrder := func(value string) {
		orderMu.Lock()
		order = append(order, value)
		orderMu.Unlock()
	}

	machine.SetTransitions(
		NewTransitionItem("block", "idle", "idle", Handler[*testPayload]{Handler: func(_ *FSM[*testPayload], _ ...any) error {
			close(blockStarted)
			<-releaseBlock
			appendOrder("direct")
			return nil
		}}),
		NewTransitionItem("low", "idle", "idle", Handler[*testPayload]{Handler: func(_ *FSM[*testPayload], _ ...any) error {
			appendOrder("low")
			return nil
		}}),
		NewTransitionItem("high", "idle", "idle", Handler[*testPayload]{Handler: func(_ *FSM[*testPayload], _ ...any) error {
			appendOrder("high")
			return nil
		}}),
	)

	directDone := make(chan error, 1)
	go func() {
		_, err := machine.ProcessEvent("block")
		directDone <- err
	}()
	waitSignal(t, blockStarted, "direct event start")

	lowReceipt, lowNotify, lowCancel := submitTrackedEvent(t, machine, "low", WithEventPriority(EventPriorityLow))
	defer lowCancel()
	highReceipt, highNotify, highCancel := submitTrackedEvent(t, machine, "high", WithEventPriority(EventPriorityHigh))
	defer highCancel()
	close(releaseBlock)

	if err := <-directDone; err != nil {
		t.Fatalf("direct event error = %v", err)
	}
	highResult := waitReceipt(t, highReceipt)
	lowResult := waitReceipt(t, lowReceipt)
	if highResult.Status != AsyncEventSucceeded || lowResult.Status != AsyncEventSucceeded {
		t.Fatalf("unexpected results: high=%+v low=%+v", highResult, lowResult)
	}
	if notified := waitNotification(t, highNotify); notified.ID != highResult.ID {
		t.Fatalf("high notification = %+v", notified)
	}
	if notified := waitNotification(t, lowNotify); notified.ID != lowResult.ID {
		t.Fatalf("low notification = %+v", notified)
	}

	orderMu.Lock()
	defer orderMu.Unlock()
	if len(order) != 3 || order[0] != "direct" || order[1] != "high" || order[2] != "low" {
		t.Fatalf("event order = %v, want [direct high low]", order)
	}
}

func TestDirectProcessEventHasPriorityOverQueuedHighEvent(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	defer func() { _ = machine.Close() }()
	var mu sync.Mutex
	var order []string
	machine.SetTransitions(
		NewTransitionItem("direct", "idle", "idle", Handler[*testPayload]{Handler: func(_ *FSM[*testPayload], _ ...any) error {
			mu.Lock()
			order = append(order, "direct")
			mu.Unlock()
			return nil
		}}),
		NewTransitionItem("queued", "idle", "idle", Handler[*testPayload]{Handler: func(_ *FSM[*testPayload], _ ...any) error {
			mu.Lock()
			order = append(order, "queued")
			mu.Unlock()
			return nil
		}}),
	)

	machine.eventScheduleLock.Lock()
	directDone := make(chan error, 1)
	go func() {
		_, err := machine.ProcessEvent("direct")
		directDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	for machine.directEventPending.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if machine.directEventPending.Load() == 0 {
		machine.eventScheduleLock.Unlock()
		t.Fatal("direct event did not enter scheduling")
	}
	receipt, _, cancel := submitTrackedEvent(t, machine, "queued", WithEventPriority(EventPriorityHigh))
	defer cancel()
	machine.eventScheduleLock.Unlock()

	if err := <-directDone; err != nil {
		t.Fatalf("direct event error = %v", err)
	}
	if result := waitReceipt(t, receipt); result.Status != AsyncEventSucceeded {
		t.Fatalf("queued result = %+v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "direct" || order[1] != "queued" {
		t.Fatalf("event order = %v, want [direct queued]", order)
	}
}

func TestSubmitEventWithoutContextAndNotifyIsFireAndForget(t *testing.T) {
	machine, releaseBlock := newBlockedFSM(t)
	ran := make(chan struct{})
	machine.SetTransitions(NewTransitionItem("fire", "idle", "idle", Handler[*testPayload]{Handler: func(fsm *FSM[*testPayload], _ ...any) error {
		if fsm.GetEventContext().Done() != nil {
			t.Error("fire-and-forget event unexpectedly received a cancelable context")
		}
		close(ran)
		return nil
	}}))

	receipt, err := machine.SubmitEvent("fire")
	if err != nil {
		close(releaseBlock)
		t.Fatal(err)
	}
	if receipt != nil {
		close(releaseBlock)
		t.Fatalf("fire-and-forget receipt = %+v, want nil", receipt)
	}
	waitPendingCount(t, machine, 1, 0)
	close(releaseBlock)
	waitSignal(t, ran, "fire-and-forget handler")
	if err := machine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAsyncEventExpiresWhileWaitingAndIsRemoved(t *testing.T) {
	machine, releaseBlock := newBlockedFSM(t)
	defer func() {
		close(releaseBlock)
		_ = machine.Close()
	}()

	receipt, notify, cancel := submitTrackedEvent(
		t, machine, "queued",
		WithEventPriority(EventPriorityLow),
		WithEventTTL(20*time.Millisecond),
	)
	defer cancel()
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventExpired || !errors.Is(result.Error, context.DeadlineExceeded) {
		t.Fatalf("expired result = %+v", result)
	}
	if !result.StartedAt.IsZero() {
		t.Fatalf("expired queued event unexpectedly started at %v", result.StartedAt)
	}
	if high, low := machine.PendingEventCount(); high != 0 || low != 0 {
		t.Fatalf("pending count after expiration = (%d, %d)", high, low)
	}
	if notified := waitNotification(t, notify); notified.Status != AsyncEventExpired {
		t.Fatalf("expiration notification = %+v", notified)
	}
}

func TestFireAndForgetExpirationRemovesEventWithoutRunningHandler(t *testing.T) {
	machine, releaseBlock := newBlockedFSM(t)
	var ran atomic.Bool
	machine.SetTransitions(NewTransitionItem("expire", "idle", "idle", Handler[*testPayload]{Handler: func(_ *FSM[*testPayload], _ ...any) error {
		ran.Store(true)
		return nil
	}}))

	receipt, err := machine.SubmitEvent("expire", WithEventPriority(EventPriorityLow), WithEventTTL(20*time.Millisecond))
	if err != nil {
		close(releaseBlock)
		t.Fatal(err)
	}
	if receipt != nil {
		close(releaseBlock)
		t.Fatal("fire-and-forget expiration returned a receipt")
	}
	time.Sleep(40 * time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_, low := machine.PendingEventCount()
		if low == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, low := machine.PendingEventCount(); low != 0 {
		close(releaseBlock)
		t.Fatalf("expired fire-and-forget event remains queued: low=%d", low)
	}
	close(releaseBlock)
	if err := machine.Close(); err != nil {
		t.Fatal(err)
	}
	if ran.Load() {
		t.Fatal("expired fire-and-forget handler was executed")
	}
}

func TestAsyncEventExpirationDoesNotCancelRunningHandler(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	defer func() { _ = machine.Close() }()
	started := make(chan struct{})
	release := make(chan struct{})
	machine.SetTransitions(NewTransitionItem("run", "idle", "done", Handler[*testPayload]{Handler: func(fsm *FSM[*testPayload], _ ...any) error {
		if _, hasDeadline := fsm.GetEventContext().Deadline(); hasDeadline {
			return errors.New("queue expiration leaked into running context")
		}
		close(started)
		<-release
		return fsm.GetEventContext().Err()
	}}))

	expiresAt := time.Now().Add(100 * time.Millisecond)
	receipt, notify, cancel := submitTrackedEvent(t, machine, "run", WithEventExpiresAt(expiresAt))
	defer cancel()
	waitSignal(t, started, "running event start")
	if delay := time.Until(expiresAt) + 20*time.Millisecond; delay > 0 {
		time.Sleep(delay)
	}
	close(release)
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventSucceeded || result.Error != nil || machine.GetCurrentState() != "done" {
		t.Fatalf("running event was affected by expiration: state=%q result=%+v", machine.GetCurrentState(), result)
	}
	if notified := waitNotification(t, notify); notified.Status != AsyncEventSucceeded {
		t.Fatalf("running notification = %+v", notified)
	}
}

func TestQueuedContextCancellationRemovesEventAndNotifies(t *testing.T) {
	machine, releaseBlock := newBlockedFSM(t)
	defer func() {
		close(releaseBlock)
		_ = machine.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	notify := make(chan AsyncEventResult, 1)
	receipt, err := machine.SubmitEvent("queued", WithEventContext(ctx), WithEventNotify(notify))
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventCanceled || !errors.Is(result.Error, context.Canceled) {
		t.Fatalf("canceled result = %+v", result)
	}
	if high, low := machine.PendingEventCount(); high != 0 || low != 0 {
		t.Fatalf("pending count after cancellation = (%d, %d)", high, low)
	}
	if notified := waitNotification(t, notify); notified.ID != receipt.ID || notified.Status != AsyncEventCanceled {
		t.Fatalf("notification = %+v", notified)
	}
}

func TestRunningAsyncEventObservesContextCancellation(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	defer func() { _ = machine.Close() }()
	started := make(chan struct{})
	machine.SetTransitions(NewTransitionItem("wait", "idle", "done", Handler[*testPayload]{
		Handler: func(fsm *FSM[*testPayload], _ ...any) error {
			close(started)
			<-fsm.GetEventContext().Done()
			return nil
		},
	}))

	ctx, cancel := context.WithCancel(context.Background())
	notify := make(chan AsyncEventResult, 1)
	receipt, err := machine.SubmitEvent("wait", WithEventContext(ctx), WithEventNotify(notify))
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started, "async handler start")
	cancel()

	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventCanceled || !errors.Is(result.Error, context.Canceled) {
		t.Fatalf("running cancellation result = %+v", result)
	}
	if machine.GetCurrentState() != "idle" || result.Trace.TransitionResult {
		t.Fatalf("canceled event committed state: state=%q trace=%+v", machine.GetCurrentState(), result.Trace)
	}
	if notified := waitNotification(t, notify); notified.Status != AsyncEventCanceled {
		t.Fatalf("running cancellation notification = %+v", notified)
	}
}

func TestCloseCancelsRunningTrackedEvent(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	started := make(chan struct{})
	machine.SetTransitions(NewTransitionItem("wait", "idle", "done", Handler[*testPayload]{
		Handler: func(fsm *FSM[*testPayload], _ ...any) error {
			close(started)
			<-fsm.GetEventContext().Done()
			return nil
		},
	}))

	receipt, notify, _ := submitTrackedEvent(t, machine, "wait")
	waitSignal(t, started, "async handler start")
	if err := machine.Close(); err != nil {
		t.Fatal(err)
	}
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventCanceled || !errors.Is(result.Error, context.Canceled) {
		t.Fatalf("Close cancellation result = %+v", result)
	}
	if notified := waitNotification(t, notify); notified.Status != AsyncEventCanceled {
		t.Fatalf("Close notification = %+v", notified)
	}
}

func TestCloseWaitsForRunningFireAndForgetEvent(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	started := make(chan struct{})
	release := make(chan struct{})
	machine.SetTransitions(NewTransitionItem("wait", "idle", "done", Handler[*testPayload]{Handler: func(_ *FSM[*testPayload], _ ...any) error {
		close(started)
		<-release
		return nil
	}}))

	receipt, err := machine.SubmitEvent("wait")
	if err != nil || receipt != nil {
		t.Fatalf("fire-and-forget submit = (%+v, %v)", receipt, err)
	}
	waitSignal(t, started, "fire-and-forget handler start")
	closed := make(chan struct{})
	go func() {
		_ = machine.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned before fire-and-forget handler exited")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	waitSignal(t, closed, "queue close")
	if machine.GetCurrentState() != "done" {
		t.Fatalf("fire-and-forget state = %q", machine.GetCurrentState())
	}
}

func TestCloseCancelsTrackedEventQueuedBehindDirectProcessing(t *testing.T) {
	machine, releaseBlock := newBlockedFSM(t)
	receipt, notify, _ := submitTrackedEvent(t, machine, "queued")

	closed := make(chan struct{})
	go func() {
		_ = machine.Close()
		close(closed)
	}()
	waitSignal(t, closed, "queue close behind direct event")
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventCanceled || !errors.Is(result.Error, ErrorEventQueueClosed) {
		t.Fatalf("queued close result = %+v", result)
	}
	if notified := waitNotification(t, notify); notified.Status != AsyncEventCanceled {
		t.Fatalf("queued close notification = %+v", notified)
	}
	close(releaseBlock)
}

func TestAsyncEventOptionValidationAndPairing(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	defer func() { _ = machine.Close() }()
	if _, err := machine.SubmitEvent("event", WithEventPriority(99)); !errors.Is(err, ErrorInvalidEventPriority) {
		t.Fatalf("invalid priority error = %v", err)
	}
	if _, err := machine.SubmitEvent("event", WithEventTTL(-time.Second)); !errors.Is(err, ErrorInvalidEventTTL) {
		t.Fatalf("invalid TTL error = %v", err)
	}
	var nilContext context.Context
	if _, err := machine.SubmitEvent("event", WithEventContext(nilContext)); !errors.Is(err, ErrorNilEventContext) {
		t.Fatalf("nil context error = %v", err)
	}
	var nilNotify chan AsyncEventResult
	if _, err := machine.SubmitEvent("event", WithEventNotify(nilNotify)); !errors.Is(err, ErrorNilEventNotify) {
		t.Fatalf("nil notify error = %v", err)
	}
	if _, err := machine.SubmitEvent("event", WithEventContext(context.Background())); !errors.Is(err, ErrorEventContextNotifyMismatch) {
		t.Fatalf("context-only error = %v", err)
	}
	if _, err := machine.SubmitEvent("event", WithEventNotify(make(chan AsyncEventResult, 1))); !errors.Is(err, ErrorEventContextNotifyMismatch) {
		t.Fatalf("notify-only error = %v", err)
	}
}

func TestAsyncEventAbsoluteExpiration(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	defer func() { _ = machine.Close() }()
	machine.SetTransitions(NewTransitionItem[*testPayload]("event", "idle", "done", Handler[*testPayload]{}))

	receipt, notify, cancel := submitTrackedEvent(t, machine, "event", WithEventExpiresAt(time.Now().Add(-time.Millisecond)))
	defer cancel()
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventExpired || !errors.Is(result.Error, context.DeadlineExceeded) {
		t.Fatalf("absolute expiration result = %+v", result)
	}
	if machine.GetCurrentState() != "idle" {
		t.Fatalf("expired event changed state to %q", machine.GetCurrentState())
	}
	if notified := waitNotification(t, notify); notified.Status != AsyncEventExpired {
		t.Fatalf("absolute expiration notification = %+v", notified)
	}
}

func TestSubmitAfterClose(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	if err := machine.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.SubmitEvent("event"); !errors.Is(err, ErrorEventQueueClosed) {
		t.Fatalf("SubmitEvent() after Close error = %v", err)
	}
}

func submitTrackedEvent(
	t *testing.T,
	machine *FSM[*testPayload],
	event FSMEvent,
	options ...AsyncEventOption,
) (*AsyncEventReceipt, <-chan AsyncEventResult, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	notify := make(chan AsyncEventResult, 1)
	options = append(options, WithEventContext(ctx), WithEventNotify(notify))
	receipt, err := machine.SubmitEvent(event, options...)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if receipt == nil {
		cancel()
		t.Fatal("tracked event receipt = nil")
	}
	return receipt, notify, cancel
}

func newBlockedFSM(t *testing.T) (*FSM[*testPayload], chan struct{}) {
	t.Helper()
	machine := NewFSM("test", "idle", &testPayload{})
	started := make(chan struct{})
	release := make(chan struct{})
	machine.SetTransitions(
		NewTransitionItem("block", "idle", "idle", Handler[*testPayload]{Handler: func(_ *FSM[*testPayload], _ ...any) error {
			close(started)
			<-release
			return nil
		}}),
		NewTransitionItem[*testPayload]("queued", "idle", "idle", Handler[*testPayload]{}),
	)
	go func() { _, _ = machine.ProcessEvent("block") }()
	waitSignal(t, started, "blocking event start")
	return machine, release
}

func waitReceipt(t *testing.T, receipt *AsyncEventReceipt) AsyncEventResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := receipt.Wait(ctx)
	if err != nil {
		t.Fatalf("receipt.Wait() error = %v", err)
	}
	return result
}

func waitNotification(t *testing.T, notify <-chan AsyncEventResult) AsyncEventResult {
	t.Helper()
	select {
	case result := <-notify:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event notification")
		return AsyncEventResult{}
	}
}

func waitPendingCount(t *testing.T, machine *FSM[*testPayload], wantHigh, wantLow int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		high, low := machine.PendingEventCount()
		if high == wantHigh && low == wantLow {
			return
		}
		time.Sleep(time.Millisecond)
	}
	high, low := machine.PendingEventCount()
	t.Fatalf("pending count = (%d, %d), want (%d, %d)", high, low, wantHigh, wantLow)
}

func waitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
