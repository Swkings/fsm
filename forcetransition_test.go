package fsm

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestForceTransitionUpdatesStatePayloadAndTrace(t *testing.T) {
	payload := &testPayload{Value: 3}
	machine := NewFSM("test", "idle", payload)
	previousID := machine.TtId

	trace, err := machine.ForceTransition(
		"maintenance",
		func(current *testPayload) error {
			current.Value = 42

			return nil
		},
	)
	if err != nil {
		t.Fatalf("ForceTransition() error = %v", err)
	}
	if machine.GetCurrentState() != "maintenance" || payload.Value != 42 {
		t.Fatalf(
			"state=%q payload=%+v, want maintenance and value 42",
			machine.GetCurrentState(), payload,
		)
	}
	if trace.OperationBy != OperationByManual || trace.Event != EventEmpty ||
		trace.FromState != "idle" || trace.ToState != "maintenance" ||
		trace.FinalState != "maintenance" || !trace.TransitionResult ||
		trace.Error != nil {
		t.Fatalf("manual trace = %+v", trace)
	}
	if trace.PrevTtId != previousID || trace.TtId == previousID ||
		machine.TtId != trace.TtId {
		t.Fatalf(
			"transition IDs: previous=%q trace=%+v machine=%q",
			previousID, trace, machine.TtId,
		)
	}

	var entryPayload, exitPayload testPayload
	if err = json.Unmarshal(trace.EntryMetadata.Payload, &entryPayload); err != nil {
		t.Fatalf("unmarshal entry payload: %v", err)
	}
	if err = json.Unmarshal(trace.ExitMetadata.Payload, &exitPayload); err != nil {
		t.Fatalf("unmarshal exit payload: %v", err)
	}
	if entryPayload.Value != 3 || exitPayload.Value != 42 {
		t.Fatalf(
			"trace payloads: entry=%d exit=%d, want 3 and 42",
			entryPayload.Value, exitPayload.Value,
		)
	}
}

func TestForceTransitionWithoutPayloadUpdater(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{Value: 7})

	trace, err := machine.ForceTransition("done")
	if err != nil {
		t.Fatalf("ForceTransition() error = %v", err)
	}
	if machine.GetCurrentState() != "done" || machine.Payload.Value != 7 {
		t.Fatalf(
			"state=%q payload=%+v, want done and unchanged payload",
			machine.GetCurrentState(), machine.Payload,
		)
	}
	if trace.OperationBy != OperationByManual || !trace.TransitionResult {
		t.Fatalf("manual trace = %+v", trace)
	}
}

func TestForceTransitionPayloadUpdaterErrorRejectsStateCommit(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{Value: 7})
	previousID := machine.TtId
	wantErr := errors.New("payload update failed")

	trace, err := machine.ForceTransition(
		"done", func(_ *testPayload) error { return wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ForceTransition() error = %v, want %v", err, wantErr)
	}
	if machine.GetCurrentState() != "idle" || machine.TtId != previousID {
		t.Fatalf(
			"failed manual transition changed state or ID: state=%q ID=%q",
			machine.GetCurrentState(), machine.TtId,
		)
	}
	if trace.OperationBy != OperationByManual || trace.TransitionResult ||
		!errors.Is(trace.Error, wantErr) || trace.FinalState != "idle" {
		t.Fatalf("failed manual trace = %+v", trace)
	}
}

func TestProcessEventTraceOperationByAuto(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	machine.SetTransitions(
		NewTransitionItem[*testPayload](
			"start", "idle", "running", Handler[*testPayload]{},
		),
	)

	trace, err := machine.ProcessEvent("start")
	if err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	if trace.OperationBy != OperationByAuto {
		t.Fatalf("OperationBy = %q, want %q", trace.OperationBy, OperationByAuto)
	}
}
