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
	)

	directDone := make(chan error, 1)
	go func() {
		_, err := machine.ProcessEvent("direct")
		directDone <- err
	}()
	<-blockStarted

	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	notify := make(chan fsm.AsyncEventResult, 1)

	lowReceipt := mustSubmit(machine, "low",
		fsm.WithEventPriority(fsm.EventPriorityLow),
		fsm.WithEventTTL(time.Second),
	)
	highReceipt := mustSubmit(machine, "high",
		fsm.WithEventPriority(fsm.EventPriorityHigh),
		fsm.WithEventNotify(notify),
	)
	close(releaseBlock)
	if err := <-directDone; err != nil {
		panic(err)
	}

	highResult := mustWait(highReceipt, waitContext)
	lowResult := mustWait(lowReceipt, waitContext)
	notifiedResult := <-notify
	fmt.Printf("order=%v high=%s low=%s notify=%s\n",
		data.snapshot(), highResult.Status, lowResult.Status, notifiedResult.Status)

	eventContext, cancelEvent := context.WithCancel(context.Background())
	cancelEvent()
	cancelReceipt := mustSubmit(machine, "cancel", fsm.WithEventContext(eventContext))
	cancelResult := mustWait(cancelReceipt, waitContext)
	fmt.Printf("canceled event status=%s error=%v\n", cancelResult.Status, cancelResult.Error)
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

func mustWait(receipt *fsm.AsyncEventReceipt, ctx context.Context) fsm.AsyncEventResult {
	result, err := receipt.Wait(ctx)
	if err != nil {
		panic(err)
	}
	return result
}
