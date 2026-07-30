package fsm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type testPayload struct {
	Value int
}

func (payload *testPayload) JSON() ([]byte, error) { return json.Marshal(payload) }
func (payload *testPayload) DisplayName() string   { return "" }
func (payload *testPayload) Category() string      { return "" }

func TestNewFSMKeepsMutablePayloadPointer(t *testing.T) {
	payload := &testPayload{Value: 1}
	machine := NewFSM("test", "idle", payload)
	if machine.Payload != payload {
		t.Fatal("NewFSM copied or replaced the supplied payload pointer")
	}
	machine.Payload.Value = 2
	if payload.Value != 2 {
		t.Fatalf("payload value = %d, want shared mutation 2", payload.Value)
	}
}

func TestProcessEventRunsHandlerStagesInOrder(t *testing.T) {
	payload := &testPayload{}
	machine := NewFSM("test", "idle", payload)
	var stages []string
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[*testPayload]{
		BeforeHandler: func(_ *FSM[*testPayload], _ ...any) error {
			stages = append(stages, "before")

			return nil
		},
		Handler: func(fsm *FSM[*testPayload], _ ...any) error {
			stages = append(stages, "handler")
			fsm.Payload.Value++

			return nil
		},
		AfterHandler: func(_ *FSM[*testPayload], _ ...any) error {
			stages = append(stages, "after")

			return nil
		},
	}))

	trace, err := machine.ProcessEvent("start")
	if err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	if got := strings.Join(stages, ","); got != "before,handler,after" {
		t.Fatalf("handler order = %q", got)
	}
	if state := machine.GetCurrentState(); state != "running" {
		t.Fatalf("current state = %q", state)
	}
	if !trace.TransitionResult || trace.FinalState != "running" || trace.Error != nil {
		t.Fatalf("unexpected trace: %+v", trace)
	}
}

func TestMetadataReturnsCurrentFSMSnapshot(t *testing.T) {
	payload := &testPayload{Value: 3}
	machine := NewFSM("test", "idle", payload)
	metadata := machine.Metadata()

	if metadata.InstanceID != machine.InstanceID || metadata.TransitionID != machine.TtId ||
		metadata.Name != "test" || metadata.CurrentState != "idle" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	payload.Value = 9
	var snapshotPayload testPayload
	if err := json.Unmarshal(metadata.Payload, &snapshotPayload); err != nil {
		t.Fatalf("unmarshal metadata payload: %v", err)
	}
	if snapshotPayload.Value != 3 {
		t.Fatalf("metadata payload = %d, want immutable snapshot 3", snapshotPayload.Value)
	}
}

func TestProcessEventCapturesEntryAndExitMetadata(t *testing.T) {
	payload := &testPayload{Value: 3}
	machine := NewFSM("test", "idle", payload)
	entryTransitionID := machine.TtId
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[*testPayload]{
		Handler: func(fsm *FSM[*testPayload], _ ...any) error {
			fsm.Payload.Value += 7

			return nil
		},
	}))

	trace, err := machine.ProcessEvent("start")
	if err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	entry, exit := trace.EntryMetadata, trace.ExitMetadata
	if entry.CurrentState != "idle" || entry.ProcessingEvent != "start" || entry.TransitionID != entryTransitionID {
		t.Fatalf("unexpected entry metadata: %+v", entry)
	}
	if exit.CurrentState != "running" || exit.ProcessingEvent != EventEmpty ||
		exit.PreviousState != "idle" || exit.PreviousEvent != "start" || exit.TransitionID != trace.TtId {
		t.Fatalf("unexpected exit metadata: %+v", exit)
	}
	var entryPayload, exitPayload testPayload
	if err := json.Unmarshal(entry.Payload, &entryPayload); err != nil {
		t.Fatalf("unmarshal entry metadata payload: %v", err)
	}
	if err := json.Unmarshal(exit.Payload, &exitPayload); err != nil {
		t.Fatalf("unmarshal exit metadata payload: %v", err)
	}
	if entryPayload.Value != 3 || exitPayload.Value != 10 {
		t.Fatalf("metadata payloads = entry %d, exit %d", entryPayload.Value, exitPayload.Value)
	}

	payload.Value = 99
	if err := json.Unmarshal(trace.ExitMetadata.Payload, &exitPayload); err != nil || exitPayload.Value != 10 {
		t.Fatalf("exit metadata was not copied: payload=%+v error=%v", exitPayload, err)
	}
}

func TestProcessEventCapturesExitMetadataOnFailure(t *testing.T) {
	payload := &testPayload{Value: 1}
	machine := NewFSM("test", "idle", payload)
	wantErr := errors.New("handler failed")
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[*testPayload]{
		Handler: func(fsm *FSM[*testPayload], _ ...any) error {
			fsm.Payload.Value = 2

			return wantErr
		},
	}))

	trace, err := machine.ProcessEvent("start")
	if !errors.Is(err, wantErr) {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	var entryPayload, exitPayload testPayload
	if err := json.Unmarshal(trace.EntryMetadata.Payload, &entryPayload); err != nil {
		t.Fatalf("unmarshal entry metadata payload: %v", err)
	}
	if err := json.Unmarshal(trace.ExitMetadata.Payload, &exitPayload); err != nil {
		t.Fatalf("unmarshal exit metadata payload: %v", err)
	}
	if entryPayload.Value != 1 || exitPayload.Value != 2 {
		t.Fatalf("failed metadata payloads = entry %d, exit %d", entryPayload.Value, exitPayload.Value)
	}
	if trace.ExitMetadata.CurrentState != "idle" || trace.ExitMetadata.ProcessingEvent != EventEmpty || trace.FinalState != "idle" || trace.TransitionResult {
		t.Fatalf("unexpected failed trace: %+v", trace)
	}
}

func TestProcessEventPassesParamsToActorStages(t *testing.T) {
	payload := &testPayload{}
	machine := NewFSM("test", "idle", payload)
	var received []string
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[*testPayload]{
		BeforeHandler: func(_ *FSM[*testPayload], params ...any) error {
			received = append(received, fmt.Sprintf("before:%s:%d", params[0], params[1]))

			return nil
		},
		Handler: func(fsm *FSM[*testPayload], _ ...any) error {
			fsm.Payload.Value++

			return nil
		},
		AfterHandler: func(_ *FSM[*testPayload], params ...any) error {
			received = append(received, fmt.Sprintf("after:%s:%d", params[0], params[1]))

			return nil
		},
	}))

	if _, err := machine.ProcessEvent("start", "attack", 25); err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	if got := strings.Join(received, ","); got != "before:attack:25,after:attack:25" {
		t.Fatalf("received params = %q", got)
	}
	if machine.Payload.Value != 1 {
		t.Fatalf("payload value = %d, want 1", machine.Payload.Value)
	}
}

func TestProcessEventPropagatesEveryHandlerStageError(t *testing.T) {
	testError := errors.New("stage failed")
	tests := []struct {
		name        string
		beforeError error
		handlerErr  error
		afterError  error
		wantStages  string
	}{
		{name: "before", beforeError: testError, wantStages: "before"},
		{name: "handler", handlerErr: testError, wantStages: "before,handler"},
		{name: "after", afterError: testError, wantStages: "before,handler,after"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := NewFSM("test", "idle", &testPayload{})
			var stages []string
			machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[*testPayload]{
				BeforeHandler: func(_ *FSM[*testPayload], _ ...any) error {
					stages = append(stages, "before")

					return tt.beforeError
				},
				Handler: func(_ *FSM[*testPayload], _ ...any) error {
					stages = append(stages, "handler")

					return tt.handlerErr
				},
				AfterHandler: func(_ *FSM[*testPayload], _ ...any) error {
					stages = append(stages, "after")

					return tt.afterError
				},
			}))

			trace, err := machine.ProcessEvent("start")
			if !errors.Is(err, testError) {
				t.Fatalf("ProcessEvent() error = %v", err)
			}
			if got := strings.Join(stages, ","); got != tt.wantStages {
				t.Fatalf("stages = %q, want %q", got, tt.wantStages)
			}
			if machine.GetCurrentState() != "idle" {
				t.Fatalf("failed transition changed state")
			}
			if !errors.Is(trace.Error, testError) || trace.TransitionResult {
				t.Fatalf("unexpected failure trace: %+v", trace)
			}
			if machine.processingEvent != EventEmpty {
				t.Fatalf("processingEvent was not cleared")
			}
		})
	}
}

func TestInvalidEventRecordsErrorAndCleansEnvironment(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	trace, err := machine.ProcessEvent("missing")
	if err == nil {
		t.Fatal("ProcessEvent() error = nil")
	}
	if trace.Error == nil || trace.TransitionResult || trace.FinalState != "idle" {
		t.Fatalf("unexpected trace: %+v", trace)
	}
	if machine.processingEvent != EventEmpty {
		t.Fatalf("processingEvent was not cleared")
	}
}

func TestSetTransitionsValidatesBatchBeforeCommit(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	first := NewTransitionItem[*testPayload]("start", "idle", "running", Handler[*testPayload]{})
	second := NewTransitionItem[*testPayload]("start", "idle", "done", Handler[*testPayload]{})

	panicValue := capturePanic(func() { machine.SetTransitions(first, second) })
	if panicValue == nil {
		t.Fatal("SetTransitions() did not panic")
	}
	if _, ok := panicValue.(error); !ok {
		t.Fatalf("panic value type = %T, want error", panicValue)
	}
	if len(machine.transitions) != 0 || len(machine.handlers) != 0 {
		t.Fatalf("duplicate batch was partially committed")
	}
}

func TestGetEventUnderStateReturnsCopy(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	machine.SetTransitions(NewTransitionItem[*testPayload]("start", "idle", "running", Handler[*testPayload]{}))

	events := machine.GetEventUnderState("idle")
	events[0] = "changed"
	if got := machine.GetEventUnderState("idle")[0]; got != "start" {
		t.Fatalf("internal event list was modified: %q", got)
	}
}

func TestNestedProcessEventReturnsBusyInsteadOfDeadlocking(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	var nestedErr error
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[*testPayload]{
		Handler: func(fsm *FSM[*testPayload], _ ...any) error {
			_, nestedErr = fsm.ProcessEvent("nested")

			return nil
		},
	}))

	if _, err := machine.ProcessEvent("start"); err != nil {
		t.Fatalf("outer ProcessEvent() error = %v", err)
	}
	if !errors.Is(nestedErr, ErrorEventProcessing) {
		t.Fatalf("nested error = %v", nestedErr)
	}
}

func TestStateChangeDuringHandlerRejectsCommit(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[*testPayload]{
		Handler: func(fsm *FSM[*testPayload], _ ...any) error {
			fsm.SetCurrentState("external")

			return nil
		},
	}))

	trace, err := machine.ProcessEvent("start")
	if !errors.Is(err, ErrorStateChanged) {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	if machine.GetCurrentState() != "external" || trace.FinalState != "external" {
		t.Fatalf("external state change was overwritten: state=%q trace=%+v", machine.GetCurrentState(), trace)
	}
}

func TestConcurrentConfigurationProcessingAndVisualization(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	machine.SetTransitions(NewTransitionItem[*testPayload]("tick", "idle", "idle", Handler[*testPayload]{}))

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			event := FSMEvent(fmt.Sprintf("event-%d", i))
			machine.SetTransitions(NewTransitionItem[*testPayload](event, "idle", "idle", Handler[*testPayload]{}))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, err := machine.ProcessEvent("tick")
			if err != nil && !errors.Is(err, ErrorEventProcessing) {
				t.Errorf("ProcessEvent() error = %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = machine.DotString()
			_, _ = machine.VisualizeForMermaidWithGraphType(FlowChart)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = machine.GetCurrentState()
			_ = machine.GetEventUnderState("idle")
		}
	}()
	wg.Wait()
}

func TestTransitionTraceStringAndDuration(t *testing.T) {
	trace := NewTransitionTrace("previous", "current", "idle", "start").Start()
	time.Sleep(time.Millisecond)
	trace.SetFinalState("idle").TransitionFail().End(errors.New("visible error"))

	if trace.Duration() <= 0 {
		t.Fatalf("Duration() = %v", trace.Duration())
	}
	if text := trace.String(); !strings.Contains(text, "visible error") {
		t.Fatalf("String() lost error message: %s", text)
	}
}

func capturePanic(fn func()) (value any) {
	defer func() { value = recover() }()
	fn()

	return nil
}
