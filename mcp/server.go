// Package mcp provides the FSM Model Context Protocol query server.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swkings/fsm/config"
	"github.com/swkings/fsm/visualization"
)

const (
	defaultMCPServerName    = "fsm-server"
	defaultMCPServerVersion = "v1.0.0"
)

// NewServer creates an MCP server and Streamable HTTP handler backed by the supplied registry.
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
		Name: name, Version: version,
		Description: "Query registered finite state machines, runtime snapshots, and transition traces.",
	}, nil)
	annotations := &mcpsdk.ToolAnnotations{
		ReadOnlyHint:   true,
		IdempotentHint: true,
		OpenWorldHint:  boolPointer(false),
	}
	mcpsdk.AddTool(
		server,
		&mcpsdk.Tool{
			Name: "fsm_list", Description: "List FSM instances registered in this process and their current states.", Annotations: annotations,
		},
		func(_ context.Context, _ *mcpsdk.CallToolRequest, input MCPListFSMsInput) (*mcpsdk.CallToolResult, MCPListFSMsOutput, error) {
			return nil, listFSMs(registry, input), nil
		},
	)
	mcpsdk.AddTool(
		server,
		&mcpsdk.Tool{
			Name: "fsm_get", Description: "Get the complete runtime snapshot, payload, definition, available transitions, and traces of one FSM.", Annotations: annotations,
		},
		func(_ context.Context, _ *mcpsdk.CallToolRequest, input MCPGetFSMInput) (*mcpsdk.CallToolResult, MCPGetFSMOutput, error) {
			return getFSM(registry, input)
		},
	)
	mcpsdk.AddTool(
		server,
		&mcpsdk.Tool{
			Name: "fsm_traces", Description: "List the newest transition traces recorded for one registered FSM.", Annotations: annotations,
		},
		func(_ context.Context, _ *mcpsdk.CallToolRequest, input MCPListTracesInput) (*mcpsdk.CallToolResult, MCPListTracesOutput, error) {
			return listTraces(registry, input)
		},
	)

	streamable := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		&mcpsdk.StreamableHTTPOptions{
			Stateless:                    true,
			JSONResponse:                 true,
			MaxRequestBodyBytes:          cfg.MaxRequestBodyBytes,
			PropagateRequestCancellation: true,
		},
	)

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
