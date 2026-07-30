package fsm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/swkings/fsm/config"
	"github.com/swkings/fsm/store"
)

type persistentPayload struct {
	Count int `json:"count"`
}

func (payload *persistentPayload) JSON() ([]byte, error) { return json.Marshal(payload) }
func (payload *persistentPayload) DisplayName() string   { return "Persistent FSM" }
func (payload *persistentPayload) Category() string      { return "tests" }

func persistentMachine(payload *persistentPayload) *FSM[*persistentPayload] {
	return NewFSM("persistent", FSMState("idle"), payload).SetTransitions(
		NewTransitionItem("start", "idle", "running", Handler[*persistentPayload]{Handler: func(machine *FSM[*persistentPayload], _ ...any) error {
			machine.Payload.Count++

			return nil
		}}),
	)
}

func TestFSMRestoresLatestSuccessfulStoreRecord(t *testing.T) {
	backend, err := store.New(config.Store{JSONL: config.JSONL{Directory: t.TempDir(), Filename: "fsm.jsonl"}})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer func() { _ = backend.Close() }()

	first := persistentMachine(&persistentPayload{})
	if err = first.BindStore(context.Background(), backend, "player-1"); err != nil {
		t.Fatalf("bind first FSM: %v", err)
	}
	if _, err = first.ProcessEvent("start"); err != nil {
		t.Fatalf("process event: %v", err)
	}
	if _, err = first.ProcessEvent("missing"); err == nil {
		t.Fatal("missing event should fail")
	}
	instanceID, transitionID := first.InstanceID, first.TtId
	_ = first.Close()

	restartedPayload := &persistentPayload{Count: 99}
	restarted := persistentMachine(restartedPayload)
	defer func() { _ = restarted.Close() }()
	if err = restarted.BindStore(context.Background(), backend, "player-1"); err != nil {
		t.Fatalf("restore FSM: %v", err)
	}
	if restarted.GetCurrentState() != "running" || restarted.Payload.Count != 1 {
		t.Fatalf("restored state=%q payload=%+v", restarted.GetCurrentState(), restarted.Payload)
	}
	if restarted.Payload != restartedPayload {
		t.Fatal("restore replaced the payload pointer supplied to NewFSM")
	}
	if restarted.InstanceID != instanceID || restarted.TtId != transitionID {
		t.Fatalf("restored identity instance=%q transition=%q", restarted.InstanceID, restarted.TtId)
	}
}
