package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/swkings/fsm"
)

type payload struct {
	mu    sync.Mutex
	order []string
}

func (p *payload) append(event string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.order = append(p.order, event)
}

func (p *payload) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.order...)
}

func main() {
	data := &payload{}
	machine := fsm.NewFSM[payload]("async-demo", "idle", data)
	defer machine.Close()

	blockStarted := make(chan struct{})
	releaseBlock := make(chan struct{})
	fireDone := make(chan struct{})
	machine.SetTransitions(
		fsm.NewTransitionItem("direct", "idle", "idle", fsm.Handler[payload]{Handler: func(machine *fsm.FSM[payload]) error {
			close(blockStarted)
			<-releaseBlock
			machine.Payload.append("direct")
			return nil
		}}),
		fsm.NewTransitionItem("high", "idle", "idle", eventHandler("high")),
		fsm.NewTransitionItem("low", "idle", "idle", eventHandler("low")),
		fsm.NewTransitionItem("cancel", "idle", "idle", eventHandler("cancel")),
		fsm.NewTransitionItem("fire", "idle", "idle", fsm.Handler[payload]{Handler: func(*fsm.FSM[payload]) error {
			close(fireDone)
			return nil
		}}),
	)

	directDone := make(chan error, 1)
	go func() {
		_, err := machine.ProcessEvent("direct")
		directDone <- err
	}()
	<-blockStarted

	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	eventContext, cancelEvents := context.WithCancel(context.Background())
	defer cancelEvents()
	lowNotify := make(chan fsm.AsyncEventResult, 1)
	highNotify := make(chan fsm.AsyncEventResult, 1)

	mustSubmit(machine, "low",
		fsm.WithEventPriority(fsm.EventPriorityLow),
		fsm.WithEventTTL(time.Second),
		fsm.WithEventContext(eventContext),
		fsm.WithEventNotify(lowNotify),
	)
	mustSubmit(machine, "high",
		fsm.WithEventPriority(fsm.EventPriorityHigh),
		fsm.WithEventContext(eventContext),
		fsm.WithEventNotify(highNotify),
	)
	close(releaseBlock)
	if err := <-directDone; err != nil {
		panic(err)
	}

	highResult := mustNotify(highNotify, waitContext)
	lowResult := mustNotify(lowNotify, waitContext)
	fmt.Printf("order=%v high=%s low=%s\n", data.snapshot(), highResult.Status, lowResult.Status)

	cancelContext, cancelEvent := context.WithCancel(context.Background())
	cancelNotify := make(chan fsm.AsyncEventResult, 1)
	cancelEvent()
	mustSubmit(machine, "cancel",
		fsm.WithEventContext(cancelContext),
		fsm.WithEventNotify(cancelNotify),
	)
	cancelResult := mustNotify(cancelNotify, waitContext)
	fmt.Printf("canceled event status=%s error=%v\n", cancelResult.Status, cancelResult.Error)

	fireReceipt := mustSubmit(machine, "fire")
	<-fireDone
	fmt.Printf("fire-and-forget receipt=%v\n", fireReceipt)
}

func eventHandler(name string) fsm.Handler[payload] {
	return fsm.Handler[payload]{Handler: func(machine *fsm.FSM[payload]) error {
		if err := machine.GetEventContext().Err(); err != nil {
			return err
		}
		machine.Payload.append(name)
		return nil
	}}
}

func mustSubmit(machine *fsm.FSM[payload], event fsm.FSMEvent, options ...fsm.AsyncEventOption) *fsm.AsyncEventReceipt {
	receipt, err := machine.SubmitEvent(event, options...)
	if err != nil {
		panic(err)
	}
	return receipt
}

func mustNotify(notify <-chan fsm.AsyncEventResult, ctx context.Context) fsm.AsyncEventResult {
	select {
	case result := <-notify:
		return result
	case <-ctx.Done():
		panic(ctx.Err())
	}
}
