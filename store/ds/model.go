// Package ds contains persistence data structures without storage behavior.
package ds

import (
	"time"

	commonds "github.com/swkings/fsm/ds"
)

// Metadata reuses the canonical string-based wire snapshot.
type Metadata = commonds.WireMetadata

// Transition reuses the canonical FSM transition model.
type Transition = commonds.Transition

// Trace is the stable persistence representation of a transition trace.
type Trace struct {
	TransitionID string               `json:"transitionId"`
	PreviousID   string               `json:"previousId,omitempty"`
	FromState    string               `json:"fromState"`
	Event        string               `json:"event"`
	ToState      string               `json:"toState"`
	OperationBy  commonds.OperationBy `json:"operationBy"`
	FinalState   string               `json:"finalState"`
	Success      bool                 `json:"success"`
	Error        string               `json:"error,omitempty"`
	Entry        Metadata             `json:"entry"`
	Exit         Metadata             `json:"exit"`
	BeginTime    int64                `json:"beginTime"`
	EndTime      int64                `json:"endTime"`
}

// Record is one durable FSM event-processing record.
type Record struct {
	ID          string       `json:"id"`
	Key         string       `json:"key"`
	DisplayName string       `json:"displayName,omitempty"`
	Category    string       `json:"category,omitempty"`
	RecordedAt  time.Time    `json:"recordedAt"`
	Success     bool         `json:"success"`
	Metadata    Metadata     `json:"metadata"`
	Transitions []Transition `json:"transitions,omitempty"`
	Trace       Trace        `json:"trace"`
}

// Query filters records returned by a IStore.
type Query struct {
	Key            string
	Limit          int
	SuccessfulOnly bool
}

// Descriptor identifies the configured storage backend.
type Descriptor struct {
	Driver   string `json:"driver"`
	Location string `json:"location"`
}

// RemoteDirectoryEntry describes one entry returned by an SSH directory listing.
type RemoteDirectoryEntry struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Directory bool      `json:"directory"`
	Size      int64     `json:"size,omitempty"`
	Modified  time.Time `json:"modified,omitempty"`
}

// RemoteDirectoryView is the serializable result of browsing an SSH directory.
type RemoteDirectoryView struct {
	Path    string                 `json:"path"`
	Parent  string                 `json:"parent,omitempty"`
	Entries []RemoteDirectoryEntry `json:"entries"`
}
