package fsm

import (
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

func TestProcessEventRunsHandlerStagesInOrder(t *testing.T) {
	payload := &testPayload{}
	machine := NewFSM[testPayload]("test", "idle", payload)
	var stages []string
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[testPayload]{
		BeforeHandler: func(*FSM[testPayload]) error {
			stages = append(stages, "before")
			return nil
		},
		Handler: func(fsm *FSM[testPayload]) error {
			stages = append(stages, "handler")
			fsm.Payload.Value++
			return nil
		},
		AfterHandler: func(*FSM[testPayload]) error {
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
			machine := NewFSM[testPayload]("test", "idle")
			var stages []string
			machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[testPayload]{
				BeforeHandler: func(*FSM[testPayload]) error {
					stages = append(stages, "before")
					return tt.beforeError
				},
				Handler: func(*FSM[testPayload]) error {
					stages = append(stages, "handler")
					return tt.handlerErr
				},
				AfterHandler: func(*FSM[testPayload]) error {
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
	machine := NewFSM[testPayload]("test", "idle")
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
	machine := NewFSM[testPayload]("test", "idle")
	first := NewTransitionItem[testPayload]("start", "idle", "running", Handler[testPayload]{})
	second := NewTransitionItem[testPayload]("start", "idle", "done", Handler[testPayload]{})

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
	machine := NewFSM[testPayload]("test", "idle")
	machine.SetTransitions(NewTransitionItem[testPayload]("start", "idle", "running", Handler[testPayload]{}))

	events := machine.GetEventUnderState("idle")
	events[0] = "changed"
	if got := machine.GetEventUnderState("idle")[0]; got != "start" {
		t.Fatalf("internal event list was modified: %q", got)
	}
}

func TestNestedProcessEventReturnsBusyInsteadOfDeadlocking(t *testing.T) {
	machine := NewFSM[testPayload]("test", "idle")
	var nestedErr error
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[testPayload]{
		Handler: func(fsm *FSM[testPayload]) error {
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
	machine := NewFSM[testPayload]("test", "idle")
	machine.SetTransitions(NewTransitionItem("start", "idle", "running", Handler[testPayload]{
		Handler: func(fsm *FSM[testPayload]) error {
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
	machine := NewFSM[testPayload]("test", "idle")
	machine.SetTransitions(NewTransitionItem[testPayload]("tick", "idle", "idle", Handler[testPayload]{}))

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			event := FSMEvent(fmt.Sprintf("event-%d", i))
			machine.SetTransitions(NewTransitionItem[testPayload](event, "idle", "idle", Handler[testPayload]{}))
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
