package fsm

import (
	"bytes"
	"fmt"
	"os"
)

const highlightingColor = "#00AA00"

// MermaidDiagramType the type of the mermaid diagram type
type MermaidDiagramType string

const (
	// FlowChart the diagram type for output in flowchart style (https://mermaid-js.github.io/mermaid/#/flowchart) (including current state)
	FlowChart MermaidDiagramType = "flowChart"
	// StateDiagram the diagram type for output in stateDiagram style (https://mermaid-js.github.io/mermaid/#/stateDiagram)
	StateDiagram MermaidDiagramType = "stateDiagram"
)

// VisualizeForMermaidWithGraphType outputs a visualization of a FSM in Mermaid format as specified by the graphType.
func (fsm *FSM[T]) VisualizeForMermaidWithGraphType(graphType MermaidDiagramType) (string, error) {
	switch graphType {
	case FlowChart:
		return fsm.visualizeForMermaidAsFlowChart(), nil
	case StateDiagram:
		return fsm.visualizeForMermaidAsStateDiagram(), nil
	default:
		return "", fmt.Errorf("unknown MermaidDiagramType: %s", graphType)
	}
}

func (fsm *FSM[T]) visualizeForMermaidAsStateDiagram() string {
	var buf bytes.Buffer

	sortedTransitionKeys := getSortedTransitionKeys(fsm.transitions)

	buf.WriteString("```mermaid\n")
	buf.WriteString("stateDiagram-v2\n")
	buf.WriteString(fmt.Sprintln(`    [*] -->`, fsm.currentState))

	for _, k := range sortedTransitionKeys {
		v := fsm.transitions[k]
		buf.WriteString(fmt.Sprintf(`    %s --> %s: %s`, k.FromState, v, k.Event))
		buf.WriteString("\n")
	}

	buf.WriteString("```\n")

	return buf.String()
}

// visualizeForMermaidAsFlowChart outputs a visualization of a FSM in Mermaid format (including highlighting of current state).
func (fsm *FSM[T]) visualizeForMermaidAsFlowChart() string {
	var buf bytes.Buffer

	sortedTransitionKeys := getSortedTransitionKeys(fsm.transitions)
	sortedStates, statesToIDMap := getSortedStates(fsm.transitions)

	writeFlowChartGraphType(&buf)
	writeFlowChartStates(&buf, sortedStates, statesToIDMap)
	writeFlowChartTransitions(&buf, fsm.transitions, sortedTransitionKeys, statesToIDMap)
	writeFlowChartHighlightCurrent(&buf, string(fsm.currentState), statesToIDMap)

	return buf.String()
}

func writeFlowChartGraphType(buf *bytes.Buffer) {
	buf.WriteString("graph LR\n")
}

func writeFlowChartStates(buf *bytes.Buffer, sortedStates []FSMState, statesToIDMap map[string]string) {
	for _, state := range sortedStates {
		buf.WriteString(fmt.Sprintf(`    %s[%s]`, statesToIDMap[string(state)], state))
		buf.WriteString("\n")
	}

	buf.WriteString("\n")
}

func writeFlowChartTransitions(buf *bytes.Buffer, transitions map[EventStateKey]FSMState, sortedTransitionKeys []EventStateKey, statesToIDMap map[string]string) {
	for _, transition := range sortedTransitionKeys {
		target := transitions[transition]
		buf.WriteString(fmt.Sprintf(`    %s --> |%s| %s`, statesToIDMap[string(transition.FromState)], transition.Event, statesToIDMap[string(target)]))
		buf.WriteString("\n")
	}
	buf.WriteString("\n")
}

func writeFlowChartHighlightCurrent(buf *bytes.Buffer, current string, statesToIDMap map[string]string) {
	buf.WriteString(fmt.Sprintf(`    style %s fill:%s`, statesToIDMap[current], highlightingColor))
	buf.WriteString("\n")
}

func (fsm *FSM[T]) VisualizeMermaid(outFile string) error {
	codeStr, _ := fsm.VisualizeForMermaidWithGraphType(StateDiagram)
	file, _ := os.OpenFile(outFile, os.O_CREATE|os.O_RDWR, 0666)
	defer file.Close()

	n, err := file.WriteString(codeStr)
	fmt.Printf("n: %v, err: %v\n", n, err)

	return err
}
