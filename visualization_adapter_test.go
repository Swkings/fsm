package fsm

import (
	"context"
	"testing"
	"time"

	"github.com/swkings/fsm/visualization"
)

func TestFSMRegisterVisualizationPushesRuntimeUpdates(t *testing.T) {
	server := visualization.NewServer(visualization.WithTraceLimit(2))
	machine := NewFSM("door", FSMState("closed"), &struct {
		Count int `json:"count"`
	}{})
	defer machine.Close()
	machine.SetTransitions(NewTransitionItem("open", "closed", "opened", Handler[struct {
		Count int `json:"count"`
	}]{Handler: func(current *FSM[struct {
		Count int `json:"count"`
	}]) error {
		current.Payload.Count++
		return nil
	}}))

	registration, err := machine.RegisterVisualization(context.Background(), server,
		visualization.WithRegistrationID("door-1"),
		visualization.WithPath("production/doors"),
	)
	if err != nil {
		t.Fatalf("register FSM: %v", err)
	}
	if registration.Mode() != visualization.ModePush {
		t.Fatalf("mode = %q, want push", registration.Mode())
	}
	if !machine.hasRuntimeObservers() {
		t.Fatal("runtime observer was not installed")
	}

	if _, err = machine.ProcessEvent("open"); err != nil {
		t.Fatalf("process event: %v", err)
	}
	snapshot, ok := server.Snapshot("door-1")
	if !ok {
		t.Fatal("registered FSM snapshot not found")
	}
	if snapshot.Path != "production/doors" || snapshot.CurrentState != "opened" {
		t.Fatalf("snapshot = path %q, state %q", snapshot.Path, snapshot.CurrentState)
	}
	if len(snapshot.Traces) != 1 || snapshot.Traces[0].Event != "open" {
		t.Fatalf("unexpected traces: %+v", snapshot.Traces)
	}

	if !registration.Unregister() {
		t.Fatal("first unregister returned false")
	}
	select {
	case <-registration.Done():
	case <-time.After(time.Second):
		t.Fatal("registration Done was not closed")
	}
	if machine.hasRuntimeObservers() {
		t.Fatal("runtime observer was not removed")
	}
}

func TestFSMVisualizableUsesServerRegistrationProtocol(t *testing.T) {
	server := visualization.NewServer()
	machine := NewFSM("job", FSMState("idle"), &struct{}{})
	defer machine.Close()

	registration, err := server.Register(context.Background(), FSMVisualizable(machine,
		visualization.WithRegistrationID("job-1"),
		visualization.WithPath("jobs/runtime"),
	))
	if err != nil {
		t.Fatalf("register visualizable FSM: %v", err)
	}
	defer registration.Unregister()

	snapshot, ok := server.Snapshot("job-1")
	if !ok || snapshot.Path != "jobs/runtime" || snapshot.CurrentState != "idle" {
		t.Fatalf("unexpected snapshot: %+v, found=%v", snapshot, ok)
	}
}
