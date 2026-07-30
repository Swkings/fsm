// Package ds contains MCP tool input and output models without server behavior.
package ds

import (
	"time"

	visualizationds "github.com/swkings/fsm/visualization/ds"
)

type MCPListFSMsInput struct {
	Query string `json:"query,omitempty" jsonschema:"optional case-insensitive filter for FSM id, name, display name, path, or current state"`
}

type MCPGetFSMInput struct {
	ID string `json:"id" jsonschema:"required FSM instance id returned by fsm_list"`
}

type MCPListTracesInput struct {
	ID    string `json:"id" jsonschema:"required FSM instance id returned by fsm_list"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of newest traces to return; defaults to 50"`
}

type MCPFSMSummary struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	DisplayName     string    `json:"displayName"`
	Description     string    `json:"description,omitempty"`
	Path            string    `json:"path,omitempty"`
	Mode            string    `json:"mode"`
	TransitionID    string    `json:"transitionId"`
	CurrentState    string    `json:"currentState"`
	PreviousState   string    `json:"previousState,omitempty"`
	PreviousEvent   string    `json:"previousEvent,omitempty"`
	ProcessingEvent string    `json:"processingEvent,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type MCPListFSMsOutput struct {
	FSMs  []MCPFSMSummary `json:"fsms"`
	Count int             `json:"count"`
}

type MCPFSMSnapshot struct {
	MCPFSMSummary
	Payload              any                                  `json:"payload"`
	PayloadError         string                               `json:"payloadError,omitempty"`
	CollectionError      string                               `json:"collectionError,omitempty"`
	States               []string                             `json:"states"`
	Events               []string                             `json:"events"`
	Transitions          []visualizationds.TransitionSnapshot `json:"transitions"`
	AvailableTransitions []visualizationds.TransitionSnapshot `json:"availableTransitions"`
	Traces               []MCPTraceSnapshot                   `json:"traces"`
}

type MCPGetFSMOutput struct {
	FSM MCPFSMSnapshot `json:"fsm"`
}

type MCPListTracesOutput struct {
	ID     string             `json:"id"`
	Traces []MCPTraceSnapshot `json:"traces"`
	Count  int                `json:"count"`
}

type MCPMetadataSnapshot struct {
	InstanceID      string `json:"instanceId"`
	TransitionID    string `json:"transitionId"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	PreviousState   string `json:"previousState"`
	PreviousEvent   string `json:"previousEvent"`
	CurrentState    string `json:"currentState"`
	ProcessingEvent string `json:"processingEvent,omitempty"`
	Payload         any    `json:"payload"`
	PayloadError    string `json:"payloadError,omitempty"`
}

type MCPTraceSnapshot struct {
	TransitionID  string              `json:"transitionId"`
	PreviousID    string              `json:"previousId"`
	FromState     string              `json:"fromState"`
	Event         string              `json:"event"`
	ToState       string              `json:"toState"`
	FinalState    string              `json:"finalState"`
	Success       bool                `json:"success"`
	Error         string              `json:"error,omitempty"`
	EntryMetadata MCPMetadataSnapshot `json:"entryMetadata"`
	ExitMetadata  MCPMetadataSnapshot `json:"exitMetadata"`
	BeginTime     int64               `json:"beginTime"`
	EndTime       int64               `json:"endTime"`
	Duration      time.Duration       `json:"duration"`
}
