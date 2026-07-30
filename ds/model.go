// Package ds contains the dependency-free data structures shared by FSM
// runtime, persistence, visualization, and protocol adapters.
package ds

import (
	"encoding/json"
)

type (
	// FSMName identifies an FSM definition or instance.
	FSMName string
	// FSMState identifies a state in an FSM.
	FSMState string
	// FSMEvent identifies an event accepted by an FSM.
	FSMEvent string
)

// String returns the underlying FSM name.
func (name FSMName) String() string { return string(name) }

// String returns the underlying state name.
func (state FSMState) String() string { return string(state) }

// String returns the underlying event name.
func (event FSMEvent) String() string { return string(event) }

// EventStateKey uniquely identifies a transition by source state and event.
type EventStateKey struct {
	Event     FSMEvent
	FromState FSMState
}

// String returns a stable readable key used by errors and generated code.
func (key EventStateKey) String() string {
	return string(key.Event) + "_" + string(key.FromState)
}

// Metadata is an immutable-by-convention snapshot of an FSM at one instant.
type Metadata struct {
	InstanceID      string          `json:"instanceId"`
	TransitionID    string          `json:"transitionId"`
	Name            FSMName         `json:"name"`
	Description     string          `json:"description,omitempty"`
	PreviousState   FSMState        `json:"previousState"`
	PreviousEvent   FSMEvent        `json:"previousEvent"`
	CurrentState    FSMState        `json:"currentState"`
	ProcessingEvent FSMEvent        `json:"processingEvent,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	PayloadError    string          `json:"payloadError,omitempty"`
}

// Clone returns a deep copy whose payload can be modified independently.
func (metadata Metadata) Clone() Metadata {
	metadata.Payload = append(json.RawMessage(nil), metadata.Payload...)
	return metadata
}

// WireMetadata is the string-based snapshot shared by persistence and remote
// protocols. It intentionally preserves their historical public field types.
type WireMetadata struct {
	InstanceID      string          `json:"instanceId"`
	TransitionID    string          `json:"transitionId"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	PreviousState   string          `json:"previousState"`
	PreviousEvent   string          `json:"previousEvent"`
	CurrentState    string          `json:"currentState"`
	ProcessingEvent string          `json:"processingEvent,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	PayloadError    string          `json:"payloadError,omitempty"`
}

// Clone returns a deep copy whose payload can be modified independently.
func (metadata WireMetadata) Clone() WireMetadata {
	metadata.Payload = append(json.RawMessage(nil), metadata.Payload...)
	return metadata
}

// Transition describes a static state transition.
type Transition struct {
	FromState string `json:"fromState"`
	Event     string `json:"event"`
	ToState   string `json:"toState"`
}
