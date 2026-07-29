package visualization

import (
	"context"
	"testing"
	"time"
)

func TestReplayJSONLLifecycleAndDuplicateSuppression(t *testing.T) {
	hub := newReplayHub(ReplayStorageConfig{
		JSONL: ReplayJSONLConfig{
			Directory: t.TempDir(),
			Filename:  "transitions.jsonl",
		},
	})
	defer hub.close()

	snapshot := Snapshot{
		Descriptor: Descriptor{
			ID:          "player-1",
			Name:        "player",
			DisplayName: "Player One",
			Path:        "examples/player",
		},
		TransitionID:  "transition-1",
		CurrentState:  "RUN",
		PreviousState: "IDLE",
		PreviousEvent: "RUN",
		States:        []string{"IDLE", "RUN"},
		Transitions: []TransitionSnapshot{
			{FromState: "IDLE", Event: "RUN", ToState: "RUN"},
		},
		UpdatedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}

	ctx := context.Background()
	if err := hub.save(ctx, snapshot); err != nil {
		t.Fatalf("save replay snapshot: %v", err)
	}
	if err := hub.save(ctx, snapshot); err != nil {
		t.Fatalf("save duplicate replay snapshot: %v", err)
	}

	records, err := hub.list(ctx, "primary", "player-1", 10)
	if err != nil {
		t.Fatalf("list replay records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one deduplicated replay record, got %d", len(records))
	}
	record, err := hub.get(ctx, "primary", records[0].ID)
	if err != nil {
		t.Fatalf("get replay record: %v", err)
	}
	if record.Snapshot.CurrentState != "RUN" || record.Snapshot.PreviousEvent != "RUN" {
		t.Fatalf("unexpected replay snapshot: %#v", record.Snapshot)
	}

	if err := hub.remove(ctx, "primary", record.ID); err != nil {
		t.Fatalf("remove replay record: %v", err)
	}
	records, err = hub.list(ctx, "primary", "player-1", 10)
	if err != nil {
		t.Fatalf("list replay records after remove: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected replay record to be removed, got %d", len(records))
	}
}

func TestReplayDriverDefaults(t *testing.T) {
	if got := replayDriver(ReplayStorageConfig{}); got != ReplayDriverJSONL {
		t.Fatalf("empty replay config should default to jsonl, got %q", got)
	}
	if got := replayDriver(ReplayStorageConfig{MySQL: ReplayMySQLConfig{DSN: "dsn"}}); got != ReplayDriverMySQL {
		t.Fatalf("mysql dsn should select mysql, got %q", got)
	}
	if got := replayDriver(ReplayStorageConfig{Redis: ReplayRedisConfig{Address: "localhost:6379"}}); got != ReplayDriverRedis {
		t.Fatalf("redis address should select redis, got %q", got)
	}
}
