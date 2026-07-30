package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swkings/fsm/visualization"
)

func listFSMs(registry *visualization.Server, input MCPListFSMsInput) MCPListFSMsOutput {
	query := strings.ToLower(strings.TrimSpace(input.Query))
	items := make([]MCPFSMSummary, 0)
	for _, snapshot := range registry.Snapshots() {
		summary := newMCPFSMSummary(snapshot)
		if query != "" && !strings.Contains(strings.ToLower(strings.Join([]string{
			summary.ID, summary.Name, summary.DisplayName, summary.Path, summary.CurrentState,
		}, "\n")), query) {
			continue
		}
		items = append(items, summary)
	}
	return MCPListFSMsOutput{FSMs: items, Count: len(items)}
}

func getFSM(registry *visualization.Server, input MCPGetFSMInput) (*mcpsdk.CallToolResult, MCPGetFSMOutput, error) {
	snapshot, exists := registry.Snapshot(strings.TrimSpace(input.ID))
	if !exists {
		return nil, MCPGetFSMOutput{}, fmt.Errorf("fsm %q is not registered", input.ID)
	}
	return nil, MCPGetFSMOutput{FSM: newMCPFSMSnapshot(snapshot)}, nil
}

func listTraces(registry *visualization.Server, input MCPListTracesInput) (*mcpsdk.CallToolResult, MCPListTracesOutput, error) {
	id := strings.TrimSpace(input.ID)
	snapshot, exists := registry.Snapshot(id)
	if !exists {
		return nil, MCPListTracesOutput{}, fmt.Errorf("fsm %q is not registered", input.ID)
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	traces := snapshot.Traces
	if len(traces) > limit {
		traces = traces[len(traces)-limit:]
	}
	mapped := makeMCPTraceSnapshots(traces)
	return nil, MCPListTracesOutput{ID: id, Traces: mapped, Count: len(mapped)}, nil
}

func newMCPFSMSummary(snapshot visualization.Snapshot) MCPFSMSummary {
	return MCPFSMSummary{
		ID: snapshot.ID, Name: snapshot.Name, DisplayName: snapshot.DisplayName,
		Description: snapshot.Description, Path: snapshot.Path, Mode: string(snapshot.Mode),
		TransitionID: snapshot.TransitionID, CurrentState: snapshot.CurrentState,
		PreviousState: snapshot.PreviousState, PreviousEvent: snapshot.PreviousEvent,
		ProcessingEvent: snapshot.ProcessingEvent, UpdatedAt: snapshot.UpdatedAt,
	}
}

func newMCPFSMSnapshot(snapshot visualization.Snapshot) MCPFSMSnapshot {
	payloadError := ""
	if snapshot.PayloadError != nil {
		payloadError = snapshot.PayloadError.Error()
	}
	payload, payloadError := decodeMCPPayload(snapshot.Payload, payloadError)
	return MCPFSMSnapshot{
		MCPFSMSummary: newMCPFSMSummary(snapshot), Payload: payload,
		PayloadError: payloadError, CollectionError: snapshot.CollectionError,
		States: append([]string(nil), snapshot.States...), Events: append([]string(nil), snapshot.Events...),
		Transitions:          append([]visualization.TransitionSnapshot(nil), snapshot.Transitions...),
		AvailableTransitions: append([]visualization.TransitionSnapshot(nil), snapshot.AvailableTransitions...),
		Traces:               makeMCPTraceSnapshots(snapshot.Traces),
	}
}

func makeMCPTraceSnapshots(traces []visualization.TraceSnapshot) []MCPTraceSnapshot {
	result := make([]MCPTraceSnapshot, len(traces))
	for index, trace := range traces {
		result[index] = MCPTraceSnapshot{
			TransitionID: trace.TransitionID, PreviousID: trace.PreviousID,
			FromState: trace.FromState, Event: trace.Event, ToState: trace.ToState, FinalState: trace.FinalState,
			Success: trace.Success, Error: trace.Error, EntryMetadata: newMCPMetadataSnapshot(trace.EntryMetadata),
			ExitMetadata: newMCPMetadataSnapshot(trace.ExitMetadata), BeginTime: trace.BeginTime,
			EndTime: trace.EndTime, Duration: trace.Duration,
		}
	}
	return result
}

func newMCPMetadataSnapshot(metadata visualization.MetadataSnapshot) MCPMetadataSnapshot {
	payload, payloadError := decodeMCPPayload(metadata.Payload, metadata.PayloadError)
	return MCPMetadataSnapshot{
		InstanceID: metadata.InstanceID, TransitionID: metadata.TransitionID,
		Name: metadata.Name, Description: metadata.Description, PreviousState: metadata.PreviousState,
		PreviousEvent: metadata.PreviousEvent, CurrentState: metadata.CurrentState,
		ProcessingEvent: metadata.ProcessingEvent, Payload: payload, PayloadError: payloadError,
	}
}

func decodeMCPPayload(raw json.RawMessage, payloadError string) (any, string) {
	if len(raw) == 0 {
		return nil, payloadError
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		if payloadError == "" {
			payloadError = err.Error()
		}
		return string(raw), payloadError
	}
	return payload, payloadError
}
