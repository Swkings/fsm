package visualization

import (
	"context"
	"testing"
	"time"

	"github.com/swkings/fsm/config"
	"github.com/swkings/fsm/store"
)

func TestReplayReadsFSMStoreWithoutOwningPersistence(t *testing.T) {
	backend, err := store.New(config.Store{JSONL: config.JSONL{Directory: t.TempDir(), Filename: "fsm.jsonl"}})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer backend.Close()
	record := store.Record{
		ID: "record-1", Key: "player-1", DisplayName: "Player One", Category: "examples/player",
		RecordedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC), Success: true,
		Metadata:    store.Metadata{InstanceID: "instance-1", TransitionID: "transition-1", Name: "player", PreviousState: "IDLE", PreviousEvent: "RUN", CurrentState: "RUN", Payload: []byte(`{"health":100}`)},
		Transitions: []store.Transition{{FromState: "IDLE", Event: "RUN", ToState: "RUN"}},
		Trace:       store.Trace{TransitionID: "transition-1", FromState: "IDLE", Event: "RUN", ToState: "RUN", FinalState: "RUN", Success: true},
	}
	if err = backend.Save(context.Background(), record); err != nil {
		t.Fatalf("save record: %v", err)
	}

	hub := newReplayHub()
	defer hub.close()
	if err = hub.register("primary", "FSM Store", backend, true, false); err != nil {
		t.Fatalf("register replay store: %v", err)
	}
	records, err := hub.list(context.Background(), "primary", "player-1", 10)
	if err != nil || len(records) != 1 {
		t.Fatalf("list replay records: records=%+v err=%v", records, err)
	}
	replay, err := hub.get(context.Background(), "primary", record.ID)
	if err != nil {
		t.Fatalf("get replay record: %v", err)
	}
	if replay.Snapshot.CurrentState != "RUN" || replay.Snapshot.PreviousEvent != "RUN" || len(replay.Snapshot.Transitions) != 1 {
		t.Fatalf("unexpected replay snapshot: %+v", replay.Snapshot)
	}
}
