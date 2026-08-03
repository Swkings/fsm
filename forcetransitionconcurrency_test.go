package fsm

import (
	"errors"
	"testing"
)

func TestForceTransitionReturnsBusyWhileEventIsProcessing(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	machine.SetTransitions(NewTransitionItem(
		"block", "idle", "running", Handler[*testPayload]{
			Handler: func(_ *FSM[*testPayload], _ ...any) error {
				close(started)
				<-release

				return nil
			},
		},
	))
	go func() {
		_, err := machine.ProcessEvent("block")
		done <- err
	}()
	<-started

	trace, err := machine.ForceTransition("manual")
	if !errors.Is(err, ErrorEventProcessing) {
		t.Fatalf("ForceTransition() error = %v, want %v", err, ErrorEventProcessing)
	}
	if trace.OperationBy != OperationByManual || trace.ToState != "manual" ||
		trace.FinalState != "idle" || trace.TransitionResult {
		t.Fatalf("busy manual trace = %+v", trace)
	}

	close(release)
	if err = <-done; err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
}
