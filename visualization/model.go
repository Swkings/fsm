// Package visualization provides the runtime visualization protocol and server.
package visualization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/swkings/fsm/config"
	visualizationds "github.com/swkings/fsm/visualization/ds"
)

var (
	ErrServerStarted          = errors.New("visualization server already started")
	ErrServerClosed           = errors.New("visualization server is closed")
	ErrInvalidClient          = errors.New("invalid visualization client")
	ErrInvalidRegistrationID  = errors.New("invalid visualization registration id")
	ErrRegistrationIDRepeated = errors.New("visualization registration id is repeated")
	ErrInvalidPath            = errors.New("invalid visualization registration path")
	ErrRegistrationNotFound   = errors.New("visualization registration not found")
	ErrRegistrationMode       = errors.New("visualization registration mode mismatch")
)

type (
	Mode               = visualizationds.Mode
	RegistrationConfig = visualizationds.RegistrationConfig
	Descriptor         = visualizationds.Descriptor
	TransitionSnapshot = visualizationds.TransitionSnapshot
	Definition         = visualizationds.Definition
	MetadataSnapshot   = visualizationds.MetadataSnapshot
	TraceSnapshot      = visualizationds.TraceSnapshot
	RealtimeData       = visualizationds.RealtimeData
	Snapshot           = visualizationds.Snapshot
	ServerConfig       = config.VisualizationServer
)

const (
	ModePull = visualizationds.ModePull
	ModePush = visualizationds.ModePush
)

// IClient is the base contract implemented by visualization clients.
type IClient interface {
	VisualizationDefinition() Definition
	VisualizationSnapshot(context.Context) (RealtimeData, error)
}

// PublishFunc submits realtime data and any newly completed traces.
type PublishFunc func(RealtimeData, ...TraceSnapshot) error

// IPushClient extends IClient with realtime update subscriptions.
type IPushClient interface {
	IClient
	SubscribeVisualization(PublishFunc) func()
}

func normalizeRegistration(definition Definition, config RegistrationConfig, supportsPush bool) (Definition, RegistrationConfig, error) {
	definition.ID = strings.TrimSpace(definition.ID)
	if definition.ID == "" || strings.ContainsAny(definition.ID, "/\\") {
		return Definition{}, RegistrationConfig{}, fmt.Errorf("%w: %q", ErrInvalidRegistrationID, definition.ID)
	}
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.Name == "" {
		definition.Name = definition.ID
	}
	definition.Description = strings.TrimSpace(definition.Description)
	config.DisplayName = strings.TrimSpace(config.DisplayName)
	if config.DisplayName == "" {
		config.DisplayName = definition.Name
	}
	path, err := NormalizePath(config.Path)
	if err != nil {
		return Definition{}, RegistrationConfig{}, err
	}
	config.Path = path
	if config.Mode == "" {
		if supportsPush {
			config.Mode = ModePush
		} else {
			config.Mode = ModePull
		}
	}
	if config.Mode != ModePull && config.Mode != ModePush {
		return Definition{}, RegistrationConfig{}, fmt.Errorf("%w: unsupported mode %q", ErrRegistrationMode, config.Mode)
	}
	if config.Mode == ModePush && !supportsPush {
		return Definition{}, RegistrationConfig{}, fmt.Errorf("%w: client does not implement PushClient", ErrInvalidClient)
	}
	if config.Mode == ModePull {
		if config.PullInterval < 0 {
			return Definition{}, RegistrationConfig{}, fmt.Errorf("%w: pull interval must not be negative", ErrInvalidClient)
		}
		if config.PullInterval == 0 {
			config.PullInterval = 5 * time.Second
		}
	}
	completeDefinition(&definition)

	return definition, config, nil
}

func completeDefinition(definition *Definition) {
	states := make(map[string]struct{}, len(definition.States))
	events := make(map[string]struct{}, len(definition.Events))
	for _, state := range definition.States {
		if state = strings.TrimSpace(state); state != "" {
			states[state] = struct{}{}
		}
	}
	for _, event := range definition.Events {
		if event = strings.TrimSpace(event); event != "" {
			events[event] = struct{}{}
		}
	}
	for _, transition := range definition.Transitions {
		if transition.FromState != "" {
			states[string(transition.FromState)] = struct{}{}
		}
		if transition.ToState != "" {
			states[string(transition.ToState)] = struct{}{}
		}
		if transition.Event != "" {
			events[string(transition.Event)] = struct{}{}
		}
	}
	definition.States = sortedSet(states)
	definition.Events = sortedSet(events)
	definition.Transitions = append([]TransitionSnapshot(nil), definition.Transitions...)
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)

	return result
}

// NormalizePath validates and normalizes a slash-separated registration path.
func NormalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "", nil
	}
	if strings.Contains(path, "\\") {
		return "", fmt.Errorf("%w: %q", ErrInvalidPath, path)
	}
	path = strings.Trim(path, "/")
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("%w: %q", ErrInvalidPath, path)
		}
		segments[index] = segment
	}

	return strings.Join(segments, "/"), nil
}

func cloneDefinition(definition Definition) Definition {
	definition.States = append([]string(nil), definition.States...)
	definition.Events = append([]string(nil), definition.Events...)
	definition.Transitions = append([]TransitionSnapshot(nil), definition.Transitions...)

	return definition
}

func cloneTraceSnapshots(traces []TraceSnapshot) []TraceSnapshot {
	cloned := append([]TraceSnapshot(nil), traces...)
	for index := range cloned {
		cloned[index].EntryMetadata.Payload = append(json.RawMessage(nil), cloned[index].EntryMetadata.Payload...)
		cloned[index].ExitMetadata.Payload = append(json.RawMessage(nil), cloned[index].ExitMetadata.Payload...)
	}

	return cloned
}

func cloneRealtimeData(data RealtimeData) RealtimeData {
	data.Payload = append(json.RawMessage(nil), data.Payload...)
	data.Traces = cloneTraceSnapshots(data.Traces)

	return data
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Payload = append(json.RawMessage(nil), snapshot.Payload...)
	snapshot.States = append([]string(nil), snapshot.States...)
	snapshot.Events = append([]string(nil), snapshot.Events...)
	snapshot.Transitions = append([]TransitionSnapshot(nil), snapshot.Transitions...)
	snapshot.AvailableTransitions = append([]TransitionSnapshot(nil), snapshot.AvailableTransitions...)
	snapshot.Traces = cloneTraceSnapshots(snapshot.Traces)

	return snapshot
}
