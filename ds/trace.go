package ds

import (
	"encoding/json"
	"fmt"
	"time"
)

// TransitionTrace records the complete result of one event-processing attempt.
type TransitionTrace struct {
	TtId             string
	PrevTtId         string
	FromState        FSMState
	Event            FSMEvent
	ToState          FSMState
	TransitionResult bool
	FinalState       FSMState
	Error            error
	EntryMetadata    Metadata
	ExitMetadata     Metadata
	BeginTime        int64
	EndTime          int64

	beginTime time.Time
	endTime   time.Time
}

// NewTransitionTrace creates a trace. Start records its actual start time.
func NewTransitionTrace(prevID, id string, fromState FSMState, event FSMEvent) *TransitionTrace {
	return &TransitionTrace{TtId: id, PrevTtId: prevID, FromState: fromState, Event: event}
}

// Start records the transition start time.
func (trace *TransitionTrace) Start() *TransitionTrace {
	trace.beginTime = time.Now()
	trace.BeginTime = trace.beginTime.UnixNano()
	return trace
}

// End records the transition end time and the first supplied error.
func (trace *TransitionTrace) End(errs ...error) {
	if len(errs) > 0 {
		trace.Error = errs[0]
	}
	trace.endTime = time.Now()
	trace.EndTime = trace.endTime.UnixNano()
}

// Duration returns the elapsed transition duration.
func (trace *TransitionTrace) Duration() time.Duration {
	if !trace.beginTime.IsZero() && !trace.endTime.IsZero() {
		return trace.endTime.Sub(trace.beginTime)
	}
	if trace.EndTime <= trace.BeginTime {
		return 0
	}
	return time.Duration(trace.EndTime - trace.BeginTime)
}

// ReleaseControl preserves the historical chainable trace API.
func (trace *TransitionTrace) ReleaseControl() *TransitionTrace { return trace }

// SetToState records the configured target state.
func (trace *TransitionTrace) SetToState(state FSMState) *TransitionTrace {
	trace.ToState = state
	return trace
}

// SetFinalState records the actual state after processing.
func (trace *TransitionTrace) SetFinalState(state FSMState) *TransitionTrace {
	trace.FinalState = state
	return trace
}

// SetEntryMetadata records the snapshot captured before handlers run.
func (trace *TransitionTrace) SetEntryMetadata(metadata Metadata) *TransitionTrace {
	trace.EntryMetadata = metadata.Clone()
	return trace
}

// SetExitMetadata records the snapshot captured after processing finishes.
func (trace *TransitionTrace) SetExitMetadata(metadata Metadata) *TransitionTrace {
	trace.ExitMetadata = metadata.Clone()
	return trace
}

// TransitionSuccess marks the transition successful.
func (trace *TransitionTrace) TransitionSuccess() *TransitionTrace {
	trace.TransitionResult = true
	return trace
}

// TransitionFail marks the transition unsuccessful.
func (trace *TransitionTrace) TransitionFail() *TransitionTrace {
	trace.TransitionResult = false
	return trace
}

// TrimPtr returns a deep-copied value.
func (trace *TransitionTrace) TrimPtr() TransitionTrace {
	clone := *trace
	clone.EntryMetadata = trace.EntryMetadata.Clone()
	clone.ExitMetadata = trace.ExitMetadata.Clone()
	return clone
}

// String serializes the trace to its historical JSON log representation.
func (trace *TransitionTrace) String() string {
	errorMessage := ""
	if trace.Error != nil {
		errorMessage = trace.Error.Error()
	}
	view := struct {
		TtId             string   `json:"TtId"`
		PrevTtId         string   `json:"PrevTtId"`
		FromState        FSMState `json:"FromState"`
		Event            FSMEvent `json:"Event"`
		ToState          FSMState `json:"ToState"`
		TransitionResult bool     `json:"TransitionResult"`
		FinalState       FSMState `json:"FinalState"`
		Error            string   `json:"Error,omitempty"`
		EntryMetadata    Metadata `json:"EntryMetadata"`
		ExitMetadata     Metadata `json:"ExitMetadata"`
		BeginTime        int64    `json:"BeginTime"`
		EndTime          int64    `json:"EndTime"`
	}{
		TtId: trace.TtId, PrevTtId: trace.PrevTtId, FromState: trace.FromState,
		Event: trace.Event, ToState: trace.ToState, TransitionResult: trace.TransitionResult,
		FinalState: trace.FinalState, Error: errorMessage,
		EntryMetadata: trace.EntryMetadata.Clone(), ExitMetadata: trace.ExitMetadata.Clone(),
		BeginTime: trace.BeginTime, EndTime: trace.EndTime,
	}
	data, err := json.Marshal(view)
	if err != nil {
		return fmt.Sprintf("%+v", *trace)
	}
	return string(data)
}
