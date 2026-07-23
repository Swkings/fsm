package fsm

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// getSortedTransitionKeys 按源状态、事件的顺序稳定排列迁移键，保证每次生成的图文本一致。
func getSortedTransitionKeys(transitions map[EventStateKey]FSMState) []EventStateKey {
	sortedTransitionKeys := make([]EventStateKey, 0)

	for transition := range transitions {
		sortedTransitionKeys = append(sortedTransitionKeys, transition)
	}
	sort.Slice(sortedTransitionKeys, func(i, j int) bool {
		if sortedTransitionKeys[i].FromState == sortedTransitionKeys[j].FromState {
			return sortedTransitionKeys[i].Event < sortedTransitionKeys[j].Event
		}
		return sortedTransitionKeys[i].FromState < sortedTransitionKeys[j].FromState
	})

	return sortedTransitionKeys
}

// getSortedStates 收集迁移图中出现的全部状态，按名称排序，并为每个状态生成稳定的节点 ID。
func getSortedStates(transitions map[EventStateKey]FSMState) ([]FSMState, map[string]string) {
	statesToIDMap := make(map[string]string)
	for transition, target := range transitions {
		if _, ok := statesToIDMap[string(transition.FromState)]; !ok {
			statesToIDMap[string(transition.FromState)] = ""
		}
		if _, ok := statesToIDMap[string(target)]; !ok {
			statesToIDMap[string(target)] = ""
		}
	}

	sortedStates := make([]string, 0, len(statesToIDMap))
	for state := range statesToIDMap {
		sortedStates = append(sortedStates, state)
	}
	sort.Strings(sortedStates)

	for i, state := range sortedStates {
		statesToIDMap[state] = fmt.Sprintf("id%d", i)
	}
	return func() []FSMState {
		res := []FSMState{}
		for _, item := range sortedStates {
			res = append(res, FSMState(item))
		}
		return res
	}(), statesToIDMap
}

// writeHeaderLine 写入 Graphviz DOT 图的声明和默认节点样式。
func writeHeaderLine(buf *bytes.Buffer) {
	buf.WriteString(`digraph fsm {`)
	buf.WriteString(`
	node[width=1 style=filled fillcolor="darkorchid1" ]`)

	buf.WriteString("\n")
}

// writeTransitions 将迁移规则写为带事件标签的有向边。
func writeTransitions(buf *bytes.Buffer, sortedEKeys []EventStateKey, transitions map[EventStateKey]FSMState) {
	for _, k := range sortedEKeys {
		v := transitions[k]
		buf.WriteString(fmt.Sprintf(`    "%s" -> "%s" [ label = "%s" ];`, k.FromState, v, k.Event))
		buf.WriteString("\n")
	}

	buf.WriteString("\n")
}

// writeStates 写入全部状态节点，并使用红色边框标识当前状态。
func writeStates(buf *bytes.Buffer, current FSMState, sortedStateKeys []FSMState) {
	for _, k := range sortedStateKeys {
		if k == current {
			buf.WriteString(fmt.Sprintf(`    "%s" [color = "red"];`, k))
		} else {
			buf.WriteString(fmt.Sprintf(`    "%s";`, k))
		}
		buf.WriteString("\n")
	}
}

// writeFooter 写入 DOT 图的结束标记。
func writeFooter(buf *bytes.Buffer) {
	buf.WriteString(fmt.Sprintln("}"))
}

// DotString 将状态机当前配置渲染为 Graphviz DOT 文本。
// 迁移和状态会先排序，以保证相同状态机配置生成稳定、便于比较的结果；当前状态使用红色边框标识。
func (fsm *FSM[T]) DotString() string {
	var buf bytes.Buffer

	sortedEKeys := getSortedTransitionKeys(fsm.transitions)
	sortedStateKeys, _ := getSortedStates(fsm.transitions)

	writeHeaderLine(&buf)
	writeTransitions(&buf, sortedEKeys, fsm.transitions)
	writeStates(&buf, fsm.currentState, sortedStateKeys)
	writeFooter(&buf)

	return buf.String()
}

// VisualizeGraphviz 使用预设的 PNG、dot 布局和清晰度参数，将状态机渲染到 outFile。
// 运行环境必须已经安装并可执行 Graphviz 的 dot 命令。
func (fsm *FSM[T]) VisualizeGraphviz(outFile string) error {
	return fsm.VisualizeWithDetails(outFile, fsm.DotString(), "png", "dot", "72", "-Gsize=10,5 -Gdpi=200")
}

// VisualizeWithDetails 使用调用方指定的 DOT 文本、输出格式、布局、缩放和附加参数执行渲染。
// 参数会被拼接为 dot 命令并交给系统 Shell 执行，因此应只传入可信内容。
func (fsm *FSM[T]) VisualizeWithDetails(outFile string, dot string, format string, layout string, scale string, more string) error {

	cmd := fmt.Sprintf("dot -o%s -T%s -K%s -s%s %s", outFile, format, layout, scale, more)

	return system(cmd, dot)
}

// system 根据当前操作系统选择命令解释器，将 DOT 文本写入命令标准输入并等待执行结束。
func system(c string, dot string) error {

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(`cmd`, `/C`, c)
	} else {
		cmd = exec.Command(`/bin/sh`, `-c`, c)
	}
	cmd.Stdin = strings.NewReader(dot)
	return cmd.Run()

}
