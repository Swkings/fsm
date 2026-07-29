package fsm

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDotStringIncludesIsolatedCurrentStateAndEscapesText(t *testing.T) {
	current := FSMState("idle\"state")
	machine := NewFSM("test", current, &testPayload{})

	dot := machine.DotString()
	if !strings.Contains(dot, strconv.Quote(string(current))) {
		t.Fatalf("DotString() omitted or failed to escape current state:\n%s", dot)
	}
	if !strings.Contains(dot, `[color = "red"]`) {
		t.Fatalf("DotString() did not highlight current state:\n%s", dot)
	}
}

func TestMermaidUsesStableIDsAndEscapedLabels(t *testing.T) {
	current := FSMState("idle\"state")
	event := FSMEvent("start|now")
	machine := NewFSM("test", current, &testPayload{})
	machine.SetTransitions(NewTransitionItem[*testPayload](event, current, "running state", Handler[*testPayload]{}))

	flow, err := machine.VisualizeForMermaidWithGraphType(FlowChart)
	if err != nil {
		t.Fatalf("flowchart error = %v", err)
	}
	if strings.Contains(flow, "style  ") || !strings.Contains(flow, strconv.Quote(string(current))) {
		t.Fatalf("invalid flowchart output:\n%s", flow)
	}

	stateDiagram, err := machine.VisualizeForMermaidWithGraphType(StateDiagram)
	if err != nil {
		t.Fatalf("state diagram error = %v", err)
	}
	if !strings.Contains(stateDiagram, `state "idle\"state" as id`) {
		t.Fatalf("state diagram did not use escaped labels and stable IDs:\n%s", stateDiagram)
	}
}

func TestVisualizeMermaidTruncatesAndReportsFileErrors(t *testing.T) {
	machine := NewFSM("test", "idle", &testPayload{})
	outputPath := filepath.Join(t.TempDir(), "fsm.md")
	if err := os.WriteFile(outputPath, []byte(strings.Repeat("x", 4096)), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := machine.VisualizeMermaid(outputPath); err != nil {
		t.Fatalf("VisualizeMermaid() error = %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := machine.VisualizeForMermaidWithGraphType(StateDiagram)
	if string(got) != want {
		t.Fatalf("file was not truncated correctly:\ngot: %q\nwant: %q", got, want)
	}

	invalidPath := filepath.Join(t.TempDir(), "missing", "fsm.md")
	if err := machine.VisualizeMermaid(invalidPath); err == nil {
		t.Fatal("VisualizeMermaid() error = nil for missing directory")
	}
}
