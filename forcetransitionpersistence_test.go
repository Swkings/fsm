package fsm

import (
	"context"
	"testing"

	"github.com/swkings/fsm/config"
	"github.com/swkings/fsm/store"
)

func TestForceTransitionPersistsManualOperation(t *testing.T) {
	backend, err := store.New(config.Store{
		JSONL: config.JSONL{
			Directory: t.TempDir(),
			Filename:  "manual-transition.jsonl",
		},
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	machine := persistentMachine(&persistentPayload{})
	t.Cleanup(func() { _ = machine.Close() })
	if err = machine.BindStore(context.Background(), backend, "manual-1"); err != nil {
		t.Fatalf("BindStore() error = %v", err)
	}
	trace, err := machine.ForceTransition(
		"maintenance",
		func(payload *persistentPayload) error {
			payload.Count = 9

			return nil
		},
	)
	if err != nil {
		t.Fatalf("ForceTransition() error = %v", err)
	}

	record, err := backend.Latest(context.Background(), "manual-1")
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if record.Trace.OperationBy != OperationByManual ||
		record.Trace.TransitionID != trace.TtId ||
		record.Metadata.CurrentState != "maintenance" {
		t.Fatalf("persisted manual record = %+v", record)
	}
}
