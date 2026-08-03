// Package ds contains visualization protocol data without server behavior.
package ds

import (
	"encoding/json"
	"time"

	commonds "github.com/swkings/fsm/ds"
)

// Mode describes how realtime data is collected from a registered client.
type Mode string

const (
	// ModePull periodically requests snapshots from the client.
	ModePull Mode = "pull"
	// ModePush subscribes to snapshots published by the client.
	ModePush Mode = "push"
)

// RegistrationConfig controls where and how one client is registered.
type RegistrationConfig struct {
	DisplayName  string
	Path         string
	Mode         Mode
	PullInterval time.Duration
}

// Descriptor contains stable registration information.
type Descriptor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"describe,omitempty"`
	Path        string `json:"path,omitempty"`
}

// TransitionSnapshot reuses the canonical transition model.
type TransitionSnapshot = commonds.Transition

// Definition is the immutable FSM definition submitted at registration.
type Definition struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"describe,omitempty"`
	States      []string             `json:"states"`
	Events      []string             `json:"events"`
	Transitions []TransitionSnapshot `json:"transitions"`
}

// Descriptor derives a stable descriptor from the definition and registration config.
func (definition Definition) Descriptor(config RegistrationConfig) Descriptor {
	return Descriptor{
		ID: definition.ID, Name: definition.Name, DisplayName: config.DisplayName,
		Description: definition.Description, Path: config.Path,
	}
}

// MetadataSnapshot reuses the canonical string-based wire snapshot.
type MetadataSnapshot = commonds.WireMetadata

// TraceSnapshot is the visualization representation of one transition trace.
type TraceSnapshot struct {
	TransitionID  string               `json:"transitionId"`
	PreviousID    string               `json:"previousId"`
	FromState     string               `json:"fromState"`
	Event         string               `json:"event"`
	ToState       string               `json:"toState"`
	OperationBy   commonds.OperationBy `json:"operationBy"`
	FinalState    string               `json:"finalState"`
	Success       bool                 `json:"success"`
	Error         string               `json:"error,omitempty"`
	EntryMetadata MetadataSnapshot     `json:"entryMetadata"`
	ExitMetadata  MetadataSnapshot     `json:"exitMetadata"`
	BeginTime     int64                `json:"beginTime"`
	EndTime       int64                `json:"endTime"`
	Duration      time.Duration        `json:"duration"`
}

// RealtimeData is the mutable portion of a registered FSM snapshot.
type RealtimeData struct {
	TransitionID    string          `json:"transitionId"`
	CurrentState    string          `json:"currentState"`
	PreviousState   string          `json:"previousState"`
	PreviousEvent   string          `json:"previousEvent"`
	ProcessingEvent string          `json:"processingEvent,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	PayloadError    error           `json:"payloadError,omitempty"`
	Traces          []TraceSnapshot `json:"traces,omitempty"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// Snapshot is the complete API view assembled from definition and realtime data.
type Snapshot struct {
	Descriptor
	Mode                 Mode                 `json:"mode"`
	TransitionID         string               `json:"transitionId"`
	CurrentState         string               `json:"currentState"`
	PreviousState        string               `json:"previousState"`
	PreviousEvent        string               `json:"previousEvent"`
	ProcessingEvent      string               `json:"processingEvent,omitempty"`
	Payload              json.RawMessage      `json:"payload"`
	PayloadError         error                `json:"payloadError,omitempty"`
	CollectionError      string               `json:"collectionError,omitempty"`
	States               []string             `json:"states"`
	Events               []string             `json:"events"`
	Transitions          []TransitionSnapshot `json:"transitions"`
	AvailableTransitions []TransitionSnapshot `json:"availableTransitions"`
	Traces               []TraceSnapshot      `json:"traces"`
	UpdatedAt            time.Time            `json:"updatedAt"`
}
