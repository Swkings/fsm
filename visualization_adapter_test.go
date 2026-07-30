package fsm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/swkings/fsm/visualization"
)

type visualizationTestPayload struct {
	Count int `json:"count"`
}

func (p *visualizationTestPayload) JSON() ([]byte, error) { return json.Marshal(p) }
func (p *visualizationTestPayload) DisplayName() string   { return "" }
func (p *visualizationTestPayload) Category() string      { return "" }

func TestFSMImplementsPushClientDirectly(t *testing.T) {
	server := visualization.NewServerWithConfig(visualization.ServerConfig{TraceLimit: 2})
	defer func() { _ = server.Close(context.Background()) }()
	machine := NewFSM("door", FSMState("closed"), &visualizationTestPayload{})
	machine.Describe = "warehouse door state machine"
	defer func() { _ = machine.Close() }()
	machine.SetTransitions(NewTransitionItem(
		"open",
		"closed",
		"opened",
		Handler[*visualizationTestPayload]{Handler: func(current *FSM[*visualizationTestPayload], _ ...any) error {
			current.Payload.Count++

			return nil
		}},
	))
	var client visualization.PushClientI = machine
	registration, err := server.Register(context.Background(), client, visualization.RegistrationConfig{
		DisplayName: "Warehouse Door",
		Path:        "production/doors",
	})
	if err != nil {
		t.Fatalf("register FSM: %v", err)
	}
	if registration.Mode() != visualization.ModePush {
		t.Fatalf("mode = %q, want push", registration.Mode())
	}
	if !machine.hasRuntimeObservers() {
		t.Fatal("runtime observer was not installed")
	}

	machine.SetCurrentState("opened")
	stateOnlySnapshot, exists := server.Snapshot(registration.ID())
	if !exists || stateOnlySnapshot.CurrentState != "opened" || len(stateOnlySnapshot.Traces) != 0 {
		t.Fatalf("state-only push snapshot = %+v exists=%t", stateOnlySnapshot, exists)
	}
	machine.SetCurrentState("closed")

	if _, err = machine.ProcessEvent("open"); err != nil {
		t.Fatalf("process event: %v", err)
	}
	snapshot, ok := server.Snapshot(registration.ID())
	if !ok {
		t.Fatal("registered FSM snapshot not found")
	}
	if snapshot.Path != "production/doors" || snapshot.CurrentState != "opened" {
		t.Fatalf("snapshot = path %q, state %q", snapshot.Path, snapshot.CurrentState)
	}
	if snapshot.ID != machine.InstanceID || snapshot.Name != string(machine.Name) ||
		snapshot.Description != machine.Describe || snapshot.DisplayName != "Warehouse Door" {
		t.Fatalf("snapshot identity = %+v", snapshot.Descriptor)
	}
	if len(snapshot.Traces) != 1 || snapshot.Traces[0].Event != "open" {
		t.Fatalf("unexpected traces: %+v", snapshot.Traces)
	}
	entryMetadata, exitMetadata := snapshot.Traces[0].EntryMetadata, snapshot.Traces[0].ExitMetadata
	if entryMetadata.CurrentState != "closed" || exitMetadata.CurrentState != "opened" {
		t.Fatalf("trace metadata states = entry %q, exit %q", entryMetadata.CurrentState, exitMetadata.CurrentState)
	}
	var entryPayload, exitPayload visualizationTestPayload
	if err := json.Unmarshal(entryMetadata.Payload, &entryPayload); err != nil {
		t.Fatalf("unmarshal trace entry metadata payload: %v", err)
	}
	if err := json.Unmarshal(exitMetadata.Payload, &exitPayload); err != nil {
		t.Fatalf("unmarshal trace exit metadata payload: %v", err)
	}
	if entryPayload.Count != 0 || exitPayload.Count != 1 {
		t.Fatalf("trace metadata payloads = entry %d, exit %d", entryPayload.Count, exitPayload.Count)
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

type emptyPayload struct {
	Data struct{}
}

func (d *emptyPayload) JSON() ([]byte, error) { return json.Marshal(d) }
func (d *emptyPayload) DisplayName() string   { return "" }
func (d *emptyPayload) Category() string      { return "" }

func TestFSMUsesDefaultVisualizationConfig(t *testing.T) {
	server := visualization.NewServer()
	machine := NewFSM("job", FSMState("idle"), &emptyPayload{})
	defer func() { _ = machine.Close() }()

	registration, err := server.Register(context.Background(), machine, visualization.RegistrationConfig{})
	if err != nil {
		t.Fatalf("register FSM: %v", err)
	}
	defer registration.Unregister()
	if registration.ID() == "" {
		t.Fatal("default registration ID is empty")
	}
	snapshot, ok := server.Snapshot(registration.ID())
	if !ok || snapshot.Name != "job" || snapshot.CurrentState != "idle" {
		t.Fatalf("unexpected snapshot: %+v, found=%v", snapshot, ok)
	}
}
