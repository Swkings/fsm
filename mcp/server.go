// Package mcp provides the FSM Model Context Protocol query server.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swkings/fsm/config"
	"github.com/swkings/fsm/visualization"
)

const (
	defaultMCPServerName    = "fsm-server"
	defaultMCPServerVersion = "v1.0.0"
)

// MCPListFSMsInput filters registered FSMs by ID, name, display name, path or state.
type MCPListFSMsInput struct {
	Query string `json:"query,omitempty" jsonschema:"optional case-insensitive filter for FSM id, name, display name, path, or current state"`
}

// MCPGetFSMInput selects a registered FSM instance.
type MCPGetFSMInput struct {
	ID string `json:"id" jsonschema:"required FSM instance id returned by fsm_list"`
}

// MCPListTracesInput selects recent transition traces for an FSM.
type MCPListTracesInput struct {
	ID    string `json:"id" jsonschema:"required FSM instance id returned by fsm_list"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of newest traces to return; defaults to 50"`
}

// MCPFSMSummary is the compact result returned by fsm_list.
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

// MCPListFSMsOutput is the structured result of fsm_list.
type MCPListFSMsOutput struct {
	FSMs  []MCPFSMSummary `json:"fsms"`
	Count int             `json:"count"`
}

// MCPFSMSnapshot contains the complete queryable state of one FSM.
type MCPFSMSnapshot struct {
	MCPFSMSummary
	Payload              any                                `json:"payload"`
	PayloadError         string                             `json:"payloadError,omitempty"`
	CollectionError      string                             `json:"collectionError,omitempty"`
	States               []string                           `json:"states"`
	Events               []string                           `json:"events"`
	Transitions          []visualization.TransitionSnapshot `json:"transitions"`
	AvailableTransitions []visualization.TransitionSnapshot `json:"availableTransitions"`
	Traces               []MCPTraceSnapshot                 `json:"traces"`
}

type MCPGetFSMOutput struct {
	FSM MCPFSMSnapshot `json:"fsm"`
}

type MCPListTracesOutput struct {
	ID     string             `json:"id"`
	Traces []MCPTraceSnapshot `json:"traces"`
	Count  int                `json:"count"`
}

// MCPMetadataSnapshot is an FSM data snapshot with an arbitrary JSON payload.
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

// MCPTraceSnapshot is one transition trace returned through MCP.
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

// NewServer creates an MCP server and Streamable HTTP handler backed by the
// supplied live FSM registry.
func NewServer(registry *visualization.Server, cfg config.MCP) (*mcpsdk.Server, http.Handler) {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = defaultMCPServerName
	}
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = defaultMCPServerVersion
	}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:        name,
		Version:     version,
		Description: "Query registered finite state machines, runtime snapshots, and transition traces.",
	}, nil)
	annotations := &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPointer(false)}
	mcpsdk.AddTool[MCPListFSMsInput, MCPListFSMsOutput](server, &mcpsdk.Tool{
		Name: "fsm_list", Description: "List FSM instances registered in this process and their current states.", Annotations: annotations,
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input MCPListFSMsInput) (*mcpsdk.CallToolResult, MCPListFSMsOutput, error) {
		return nil, listFSMs(registry, input), nil
	})
	mcpsdk.AddTool[MCPGetFSMInput, MCPGetFSMOutput](server, &mcpsdk.Tool{
		Name: "fsm_get", Description: "Get the complete runtime snapshot, payload, definition, available transitions, and traces of one FSM.", Annotations: annotations,
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input MCPGetFSMInput) (*mcpsdk.CallToolResult, MCPGetFSMOutput, error) {
		snapshot, exists := registry.Snapshot(strings.TrimSpace(input.ID))
		if !exists {
			return nil, MCPGetFSMOutput{}, fmt.Errorf("fsm %q is not registered", input.ID)
		}
		return nil, MCPGetFSMOutput{FSM: newMCPFSMSnapshot(snapshot)}, nil
	})
	mcpsdk.AddTool[MCPListTracesInput, MCPListTracesOutput](server, &mcpsdk.Tool{
		Name: "fsm_traces", Description: "List the newest transition traces recorded for one registered FSM.", Annotations: annotations,
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input MCPListTracesInput) (*mcpsdk.CallToolResult, MCPListTracesOutput, error) {
		return listTraces(registry, input)
	})

	streamable := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		MaxRequestBodyBytes:          cfg.MaxRequestBodyBytes,
		PropagateRequestCancellation: true,
	})
	endpoint := Endpoint(cfg.Endpoint)
	mux := http.NewServeMux()
	mux.Handle(endpoint, streamable)
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "mcpEndpoint": endpoint})
	})
	return server, mux
}

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
	mcpTraces := makeMCPTraceSnapshots(traces)
	return nil, MCPListTracesOutput{ID: id, Traces: mcpTraces, Count: len(mcpTraces)}, nil
}

func newMCPFSMSummary(snapshot visualization.Snapshot) MCPFSMSummary {
	return MCPFSMSummary{
		ID: snapshot.ID, Name: snapshot.Name, DisplayName: snapshot.DisplayName, Description: snapshot.Description,
		Path: snapshot.Path, Mode: string(snapshot.Mode), TransitionID: snapshot.TransitionID,
		CurrentState: snapshot.CurrentState, PreviousState: snapshot.PreviousState, PreviousEvent: snapshot.PreviousEvent,
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
		MCPFSMSummary:        newMCPFSMSummary(snapshot),
		Payload:              payload,
		PayloadError:         payloadError,
		CollectionError:      snapshot.CollectionError,
		States:               append([]string(nil), snapshot.States...),
		Events:               append([]string(nil), snapshot.Events...),
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
			Success: trace.Success, Error: trace.Error,
			EntryMetadata: newMCPMetadataSnapshot(trace.EntryMetadata),
			ExitMetadata:  newMCPMetadataSnapshot(trace.ExitMetadata),
			BeginTime:     trace.BeginTime, EndTime: trace.EndTime, Duration: trace.Duration,
		}
	}
	return result
}

func newMCPMetadataSnapshot(metadata visualization.MetadataSnapshot) MCPMetadataSnapshot {
	payload, payloadError := decodeMCPPayload(metadata.Payload, metadata.PayloadError)
	return MCPMetadataSnapshot{
		InstanceID: metadata.InstanceID, TransitionID: metadata.TransitionID, Name: metadata.Name,
		Description: metadata.Description, PreviousState: metadata.PreviousState, PreviousEvent: metadata.PreviousEvent,
		CurrentState: metadata.CurrentState, ProcessingEvent: metadata.ProcessingEvent,
		Payload: payload, PayloadError: payloadError,
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

// Endpoint normalizes an MCP HTTP endpoint and applies the /mcp default.
func Endpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "/mcp"
	}
	endpoint = "/" + strings.Trim(endpoint, "/")
	if endpoint == "/" {
		return "/"
	}
	return endpoint
}

func boolPointer(value bool) *bool { return &value }
