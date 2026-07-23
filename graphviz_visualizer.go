package fsm

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

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

func writeHeaderLine(buf *bytes.Buffer) {
	buf.WriteString(`digraph fsm {`)
	buf.WriteString(`
	node[width=1 style=filled fillcolor="darkorchid1" ]`)

	buf.WriteString("\n")
}

func writeTransitions(buf *bytes.Buffer, sortedEKeys []EventStateKey, transitions map[EventStateKey]FSMState) {
	for _, k := range sortedEKeys {
		v := transitions[k]
		buf.WriteString(fmt.Sprintf(`    "%s" -> "%s" [ label = "%s" ];`, k.FromState, v, k.Event))
		buf.WriteString("\n")
	}

	buf.WriteString("\n")
}

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

func writeFooter(buf *bytes.Buffer) {
	buf.WriteString(fmt.Sprintln("}"))
}

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

func (fsm *FSM[T]) VisualizeGraphviz(outFile string) error {
	return fsm.VisualizeWithDetails(outFile, fsm.DotString(), "png", "dot", "72", "-Gsize=10,5 -Gdpi=200")
}

func (fsm *FSM[T]) VisualizeWithDetails(outFile string, dot string, format string, layout string, scale string, more string) error {

	cmd := fmt.Sprintf("dot -o%s -T%s -K%s -s%s %s", outFile, format, layout, scale, more)

	return system(cmd, dot)
}

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
