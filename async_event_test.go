package fsm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAsyncEventPriorityOrder(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	defer machine.Close()

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
		NewTransitionItem("block", "idle", "idle", Handler[testPayload]{Handler: func(*FSM[testPayload]) error {
			close(blockStarted)
			<-releaseBlock
			appendOrder("direct")
			return nil
		}}),
		NewTransitionItem("low", "idle", "idle", Handler[testPayload]{Handler: func(*FSM[testPayload]) error {
			appendOrder("low")
			return nil
		}}),
		NewTransitionItem("high", "idle", "idle", Handler[testPayload]{Handler: func(*FSM[testPayload]) error {
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

	lowReceipt, err := machine.SubmitEvent("low", WithEventPriority(EventPriorityLow))
	if err != nil {
		t.Fatal(err)
	}
	highReceipt, err := machine.SubmitEvent("high", WithEventPriority(EventPriorityHigh))
	if err != nil {
		t.Fatal(err)
	}
	close(releaseBlock)

	if err := <-directDone; err != nil {
		t.Fatalf("direct event error = %v", err)
	}
	highResult := waitReceipt(t, highReceipt)
	lowResult := waitReceipt(t, lowReceipt)
	if highResult.Status != AsyncEventSucceeded || lowResult.Status != AsyncEventSucceeded {
		t.Fatalf("unexpected results: high=%+v low=%+v", highResult, lowResult)
	}

	orderMu.Lock()
	defer orderMu.Unlock()
	if len(order) != 3 || order[0] != "direct" || order[1] != "high" || order[2] != "low" {
		t.Fatalf("event order = %v, want [direct high low]", order)
	}
}

func TestDirectProcessEventHasPriorityOverQueuedHighEvent(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	defer machine.Close()
	var mu sync.Mutex
	var order []string
	machine.SetTransitions(
		NewTransitionItem("direct", "idle", "idle", Handler[testPayload]{Handler: func(*FSM[testPayload]) error {
			mu.Lock()
			order = append(order, "direct")
			mu.Unlock()
			return nil
		}}),
		NewTransitionItem("queued", "idle", "idle", Handler[testPayload]{Handler: func(*FSM[testPayload]) error {
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
	receipt, err := machine.SubmitEvent("queued", WithEventPriority(EventPriorityHigh))
	if err != nil {
		machine.eventScheduleLock.Unlock()
		t.Fatal(err)
	}
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

func TestAsyncEventExpiresWhileWaiting(t *testing.T) {
	machine, releaseBlock := newBlockedFSM(t)
	defer func() {
		close(releaseBlock)
		_ = machine.Close()
	}()

	receipt, err := machine.SubmitEvent("queued", WithEventPriority(EventPriorityLow), WithEventTTL(20*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventExpired || !errors.Is(result.Error, context.DeadlineExceeded) {
		t.Fatalf("expired result = %+v", result)
	}
	if !result.StartedAt.IsZero() {
		t.Fatalf("expired queued event unexpectedly started at %v", result.StartedAt)
	}
}

func TestAsyncEventCancellationNotifiesReceiptAndChannel(t *testing.T) {
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
	select {
	case notified := <-notify:
		if notified.ID != receipt.ID || notified.Status != AsyncEventCanceled {
			t.Fatalf("notification = %+v", notified)
		}
	case <-time.After(time.Second):
		t.Fatal("notification channel was not called")
	}
}

func TestRunningAsyncEventObservesContextCancellation(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	defer machine.Close()
	started := make(chan struct{})
	machine.SetTransitions(NewTransitionItem("wait", "idle", "done", Handler[testPayload]{
		Handler: func(fsm *FSM[testPayload]) error {
			close(started)
			<-fsm.GetEventContext().Done()
			return nil
		},
	}))

	ctx, cancel := context.WithCancel(context.Background())
	receipt, err := machine.SubmitEvent("wait", WithEventContext(ctx))
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
}

func TestSubmitEventDefaultsAndQueueClose(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	machine.SetTransitions(NewTransitionItem[testPayload]("run", "idle", "done", Handler[testPayload]{}))

	receipt, err := machine.SubmitEvent("run")
	if err != nil {
		t.Fatal(err)
	}
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventSucceeded || result.Priority != EventPriorityHigh {
		t.Fatalf("default result = %+v", result)
	}
	canceledWaitContext, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	resultAgain, err := receipt.Wait(canceledWaitContext)
	if err != nil || resultAgain.ID != receipt.ID {
		t.Fatalf("completed receipt Wait() = (%+v, %v)", resultAgain, err)
	}
	if err := machine.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.SubmitEvent("run"); !errors.Is(err, ErrorEventQueueClosed) {
		t.Fatalf("SubmitEvent() after Close error = %v", err)
	}
}

func TestCloseCancelsRunningAsyncEvent(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	started := make(chan struct{})
	machine.SetTransitions(NewTransitionItem("wait", "idle", "done", Handler[testPayload]{
		Handler: func(fsm *FSM[testPayload]) error {
			close(started)
			<-fsm.GetEventContext().Done()
			return nil
		},
	}))

	receipt, err := machine.SubmitEvent("wait")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started, "async handler start")
	if err := machine.Close(); err != nil {
		t.Fatal(err)
	}
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventCanceled || !errors.Is(result.Error, context.Canceled) {
		t.Fatalf("Close cancellation result = %+v", result)
	}
}

func TestConcurrentCloseWaitsForRunningAsyncEvent(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	started := make(chan struct{})
	release := make(chan struct{})
	machine.SetTransitions(NewTransitionItem("wait", "idle", "done", Handler[testPayload]{
		Handler: func(*FSM[testPayload]) error {
			close(started)
			<-release
			return nil
		},
	}))

	receipt, err := machine.SubmitEvent("wait")
	if err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started, "async handler start")
	firstClosed := make(chan struct{})
	secondClosed := make(chan struct{})
	go func() { _ = machine.Close(); close(firstClosed) }()
	go func() { _ = machine.Close(); close(secondClosed) }()

	select {
	case <-firstClosed:
		t.Fatal("first Close returned before running handler exited")
	case <-secondClosed:
		t.Fatal("second Close returned before running handler exited")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	waitSignal(t, firstClosed, "first Close")
	waitSignal(t, secondClosed, "second Close")
	if result := waitReceipt(t, receipt); result.Status != AsyncEventCanceled {
		t.Fatalf("close result = %+v", result)
	}
}

func TestCloseCancelsEventQueuedBehindDirectProcessing(t *testing.T) {
	machine, releaseBlock := newBlockedFSM(t)
	receipt, err := machine.SubmitEvent("queued")
	if err != nil {
		close(releaseBlock)
		t.Fatal(err)
	}

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
	close(releaseBlock)
}

func TestAsyncEventOptionValidation(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	defer machine.Close()
	if _, err := machine.SubmitEvent("event", WithEventPriority(99)); !errors.Is(err, ErrorInvalidEventPriority) {
		t.Fatalf("invalid priority error = %v", err)
	}
	if _, err := machine.SubmitEvent("event", WithEventTTL(-time.Second)); !errors.Is(err, ErrorInvalidEventTTL) {
		t.Fatalf("invalid TTL error = %v", err)
	}
	if _, err := machine.SubmitEvent("event", WithEventContext(nil)); !errors.Is(err, ErrorNilEventContext) {
		t.Fatalf("nil context error = %v", err)
	}
	var notify chan AsyncEventResult
	if _, err := machine.SubmitEvent("event", WithEventNotify(notify)); !errors.Is(err, ErrorNilEventNotify) {
		t.Fatalf("nil notify error = %v", err)
	}
}

func TestAsyncEventAbsoluteExpiration(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	defer machine.Close()
	machine.SetTransitions(NewTransitionItem[testPayload]("event", "idle", "done", Handler[testPayload]{}))

	receipt, err := machine.SubmitEvent("event", WithEventExpiresAt(time.Now().Add(-time.Millisecond)))
	if err != nil {
		t.Fatal(err)
	}
	result := waitReceipt(t, receipt)
	if result.Status != AsyncEventExpired || !errors.Is(result.Error, context.DeadlineExceeded) {
		t.Fatalf("absolute expiration result = %+v", result)
	}
	if machine.GetCurrentState() != "idle" {
		t.Fatalf("expired event changed state to %q", machine.GetCurrentState())
	}
}

func newBlockedFSM(t *testing.T) (*FSM[testPayload], chan struct{}) {
	t.Helper()
	machine := NewFSM[testPayload]("test", "idle")
	started := make(chan struct{})
	release := make(chan struct{})
	machine.SetTransitions(
		NewTransitionItem("block", "idle", "idle", Handler[testPayload]{Handler: func(*FSM[testPayload]) error {
			close(started)
			<-release
			return nil
		}}),
		NewTransitionItem[testPayload]("queued", "idle", "idle", Handler[testPayload]{}),
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

func waitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
