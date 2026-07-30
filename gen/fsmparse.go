package gen

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeromicro/go-zero/core/conf"
)

const (
	eventPrefix = "Event"
	statePrefix = "State"
)

func ParseFSMData(fsmYaml FSMYaml) (FSMData, error) {
	packageDir := strings.TrimSpace(strings.TrimSuffix(fsmYaml.PackageDir, "/"))
	enginePackage := strings.TrimSpace(strings.TrimSuffix(fsmYaml.FSMEnginePackage, "/"))
	packageName := strings.TrimSpace(fsmYaml.Package)
	if packageDir == "" {
		return FSMData{}, fmt.Errorf("PackageDir is required")
	}
	if enginePackage == "" {
		return FSMData{}, fmt.Errorf("FSMEnginePackage is required")
	}
	if !token.IsIdentifier(packageName) || token.Lookup(packageName).IsKeyword() {
		return FSMData{}, fmt.Errorf("invalid Package %q", fsmYaml.Package)
	}

	states, stateSet, err := prefixedIdentifiers(fsmYaml.State, statePrefix, "state")
	if err != nil {
		return FSMData{}, err
	}
	events, eventSet, err := prefixedIdentifiers(fsmYaml.Event, eventPrefix, "event")
	if err != nil {
		return FSMData{}, err
	}

	meta := fsmYaml.FSM
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		return FSMData{}, fmt.Errorf("FSM.Name is required")
	}
	if !token.IsIdentifier(name) || token.Lookup(name).IsKeyword() {
		return FSMData{}, fmt.Errorf("invalid FSM name %q", meta.Name)
	}

	initialName := strings.TrimSpace(meta.Initial)
	if _, exists := stateSet[initialName]; !exists {
		return FSMData{}, fmt.Errorf("FSM %q uses unknown initial state %q", name, initialName)
	}

	transitions := make([]Transition, 0, len(meta.Transition))
	transitionKeys := make(map[string]struct{}, len(meta.Transition))
	for index, expression := range meta.Transition {
		parts := strings.Split(expression, "->")
		if len(parts) != 3 {
			return FSMData{}, fmt.Errorf("FSM %q transition %d must use From -> Event -> To format: %q", name, index+1, expression)
		}
		from := strings.TrimSpace(parts[0])
		event := strings.TrimSpace(parts[1])
		to := strings.TrimSpace(parts[2])
		if _, exists := stateSet[from]; !exists {
			return FSMData{}, fmt.Errorf("FSM %q transition %d uses unknown source state %q", name, index+1, from)
		}
		if _, exists := eventSet[event]; !exists {
			return FSMData{}, fmt.Errorf("FSM %q transition %d uses unknown event %q", name, index+1, event)
		}
		if _, exists := stateSet[to]; !exists {
			return FSMData{}, fmt.Errorf("FSM %q transition %d uses unknown target state %q", name, index+1, to)
		}

		key := from + "\x00" + event
		if _, exists := transitionKeys[key]; exists {
			return FSMData{}, fmt.Errorf("FSM %q contains duplicate transition for state %q and event %q", name, from, event)
		}
		transitionKeys[key] = struct{}{}
		transitions = append(transitions, Transition{
			From:      statePrefix + from,
			FromName:  from,
			Event:     eventPrefix + event,
			EventName: event,
			To:        statePrefix + to,
			ToName:    to,
		})
	}

	return FSMData{
		PackageDir:       packageDir,
		Package:          packageName,
		FSMEnginePackage: enginePackage,
		States:           states,
		Events:           events,
		FSM: FSM{
			Name:        name,
			Describe:    meta.Describe,
			Initial:     statePrefix + initialName,
			InitialName: initialName,
			Transitions: transitions,
		},
	}, nil
}

func prefixedIdentifiers(values []string, prefix string, kind string) ([]Symbol, map[string]struct{}, error) {
	result := make([]Symbol, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		identifier := prefix + value
		if value == "" || !token.IsIdentifier(value) || token.Lookup(value).IsKeyword() || !token.IsExported(value) {
			return nil, nil, fmt.Errorf(
				"invalid %s %q: enum names must be exported Go identifiers",
				kind,
				raw,
			)
		}
		if _, exists := seen[value]; exists {
			return nil, nil, fmt.Errorf("duplicate %s %q", kind, value)
		}
		seen[value] = struct{}{}
		result = append(result, Symbol{Identifier: identifier, Value: value})
	}

	return result, seen, nil
}

func LoadData(file string, v any) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read FSM description %q: %w", file, err)
	}
	if err := conf.LoadFromYamlBytes(content, v); err != nil {
		return fmt.Errorf("parse FSM description %q: %w", file, err)
	}

	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return false, fmt.Errorf("stat %q: %w", path, err)
}

func CreateDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}

	return nil
}

func CreateFSMDirLevel(path string, childLevels ...string) error {
	if err := CreateDir(path); err != nil {
		return err
	}
	for _, childLevel := range childLevels {
		childLevel = strings.Trim(childLevel, `/\`)
		if err := CreateDir(filepath.Join(path, childLevel)); err != nil {
			return err
		}
	}

	return nil
}

// Map2ListOpt 将 map 转换为切片；filter 返回 true 的元素会被跳过。
func Map2ListOpt[T comparable, V any, R any](m map[T]V, getValue func(T, V) R, filter ...func(key T, item V) bool) []R {
	result := make([]R, 0, len(m))
	for key, item := range m {
		if len(filter) > 0 && filter[0](key, item) {
			continue
		}
		result = append(result, getValue(key, item))
	}

	return result
}
