package fsm

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
)

// highlightingColor 是流程图中当前状态节点使用的高亮填充色。
const highlightingColor = "#00AA00"

// MermaidDiagramType 表示支持输出的 Mermaid 图类型。
type MermaidDiagramType string

const (
	// FlowChart 生成从左到右的流程图，并高亮显示当前状态。
	FlowChart MermaidDiagramType = "flowChart"
	// StateDiagram 生成 Mermaid stateDiagram-v2 状态图。
	StateDiagram MermaidDiagramType = "stateDiagram"
)

// VisualizeForMermaidWithGraphType 按 graphType 将当前状态机渲染为 Mermaid 文本。
// graphType 不是 FlowChart 或 StateDiagram 时返回错误。
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

// visualizeForMermaidAsStateDiagram 生成带 Mermaid Markdown 代码围栏的 stateDiagram-v2 文本。
// 图的起始箭头指向状态机的当前状态，随后按稳定顺序输出全部转移。
func (fsm *FSM[T]) visualizeForMermaidAsStateDiagram() string {
	var buf bytes.Buffer
	transitions, currentState := fsm.visualizationSnapshot()
	sortedTransitionKeys := getSortedTransitionKeys(transitions)
	sortedStates, statesToIDMap := getSortedStates(transitions, currentState)

	buf.WriteString("```mermaid\n")
	buf.WriteString("stateDiagram-v2\n")
	for _, state := range sortedStates {
		fmt.Fprintf(&buf, "    state %s as %s\n", strconv.Quote(string(state)), statesToIDMap[string(state)])
	}
	fmt.Fprintf(&buf, "    [*] --> %s\n", statesToIDMap[string(currentState)])

	for _, key := range sortedTransitionKeys {
		target := transitions[key]
		fmt.Fprintf(&buf, "    %s --> %s: %s\n", statesToIDMap[string(key.FromState)], statesToIDMap[string(target)], strconv.Quote(string(key.Event)))
	}

	buf.WriteString("```\n")

	return buf.String()
}

// visualizeForMermaidAsFlowChart 生成 Mermaid 流程图文本，并高亮当前状态节点。
func (fsm *FSM[T]) visualizeForMermaidAsFlowChart() string {
	var buf bytes.Buffer
	transitions, currentState := fsm.visualizationSnapshot()
	sortedTransitionKeys := getSortedTransitionKeys(transitions)
	sortedStates, statesToIDMap := getSortedStates(transitions, currentState)

	writeFlowChartGraphType(&buf)
	writeFlowChartStates(&buf, sortedStates, statesToIDMap)
	writeFlowChartTransitions(&buf, transitions, sortedTransitionKeys, statesToIDMap)
	writeFlowChartHighlightCurrent(&buf, string(currentState), statesToIDMap)

	return buf.String()
}

// writeFlowChartGraphType 写入从左到右的 Mermaid 流程图声明。
func writeFlowChartGraphType(buf *bytes.Buffer) {
	buf.WriteString("graph LR\n")
}

// writeFlowChartStates 按稳定顺序写入状态节点及其内部节点 ID。
func writeFlowChartStates(buf *bytes.Buffer, sortedStates []FSMState, statesToIDMap map[string]string) {
	for _, state := range sortedStates {
		fmt.Fprintf(buf, `    %s[%s]`, statesToIDMap[string(state)], strconv.Quote(string(state)))
		buf.WriteString("\n")
	}

	buf.WriteString("\n")
}

// writeFlowChartTransitions 将转移规则写为带事件标签的有向边。
func writeFlowChartTransitions(buf *bytes.Buffer, transitions map[EventStateKey]FSMState, sortedTransitionKeys []EventStateKey, statesToIDMap map[string]string) {
	for _, transition := range sortedTransitionKeys {
		target := transitions[transition]
		fmt.Fprintf(buf, `    %s -->|%s| %s`, statesToIDMap[string(transition.FromState)], strconv.Quote(string(transition.Event)), statesToIDMap[string(target)])
		buf.WriteString("\n")
	}
	buf.WriteString("\n")
}

// writeFlowChartHighlightCurrent 为当前状态对应的节点写入高亮样式。
func writeFlowChartHighlightCurrent(buf *bytes.Buffer, current string, statesToIDMap map[string]string) {
	fmt.Fprintf(buf, `    style %s fill:%s`, statesToIDMap[current], highlightingColor)
	buf.WriteString("\n")
}

// VisualizeMermaid 将 stateDiagram-v2 格式的 Mermaid Markdown 文本写入 outFile。
// 文件不存在时会创建；文件已存在时会先截断，避免残留旧内容。
func (fsm *FSM[T]) VisualizeMermaid(outFile string) error {
	code, err := fsm.VisualizeForMermaidWithGraphType(StateDiagram)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outFile, []byte(code), 0o644); err != nil {
		return fmt.Errorf("write mermaid file %q: %w", outFile, err)
	}

	return nil
}
