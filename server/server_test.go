package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swkings/fsm"
	"github.com/swkings/fsm/config"
	fsmserver "github.com/swkings/fsm/server"
	"github.com/zeromicro/go-zero/core/service"
)

type testPayload struct {
	Value int
}

func (payload *testPayload) JSON() ([]byte, error) { return json.Marshal(payload) }
func (payload *testPayload) DisplayName() string   { return "" }
func (payload *testPayload) Category() string      { return "" }

func TestManagedServerRegistersFSMAndImplementsGoZeroService(t *testing.T) {
	server := fsmserver.MustNewServer(config.Config{
		Visualization: config.Visualization{
			ServerAddress: "127.0.0.1:0",
			TraceLimit:    10,
			PathPrefix:    "tests/managed",
		},
	})
	var managedService service.Service = server
	machine := fsm.NewFSM("managed", "idle", &testPayload{})
	machine.SetTransitions(fsm.NewTransitionItem("start", "idle", "running", fsm.Handler[*testPayload]{}))
	if err := machine.Register(server); err != nil {
		t.Fatalf("RegisterTo: %v", err)
	}
	snapshot, exists := server.VisualizationServer().Snapshot(machine.InstanceID)
	if !exists || snapshot.Path != "tests/managed" || snapshot.CurrentState != "idle" {
		t.Fatalf("registered snapshot: exists=%v snapshot=%+v", exists, snapshot)
	}

	returned := startManagedServerForTest(t, server)
	if server.VisualizationURL() == "" {
		t.Fatal("visualization URL is empty after Start")
	}
	managedService.Stop()
	if recovered := <-returned; recovered != nil {
		t.Fatalf("Start panic: %v", recovered)
	}
	if err := server.Err(); err != nil {
		t.Fatalf("server Err: %v", err)
	}
	if err := machine.Close(); err != nil {
		t.Fatalf("close FSM: %v", err)
	}
}

func TestManagedServerMCPQueriesFSMWithLatestProtocol(t *testing.T) {
	server := fsmserver.MustNewServer(config.Config{
		MCP: config.MCP{
			Enabled:       true,
			ServerAddress: "127.0.0.1:0",
			Endpoint:      "/mcp",
			Name:          "fsm-test",
			Version:       "v0.0.1",
		},
	})
	machine := fsm.NewFSM("mcp-machine", "idle", &testPayload{Value: 7})
	machine.SetTransitions(fsm.NewTransitionItem("start", "idle", "running", fsm.Handler[*testPayload]{
		Handler: func(machine *fsm.FSM[*testPayload], _ ...any) error {
			machine.Payload.Value++
			return nil
		},
	}))
	if err := server.Register(machine); err != nil {
		t.Fatalf("Register: %v", err)
	}
	returned := startManagedServerForTest(t, server)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "fsm-test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint:             server.MCPURL(),
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect MCP: %v", err)
	}
	defer func() { _ = session.Close() }()
	if result := session.InitializeResult(); result == nil || result.ProtocolVersion != "2026-07-28" {
		t.Fatalf("MCP protocol = %+v", result)
	}

	listResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "fsm_list", Arguments: map[string]any{}})
	if err != nil || listResult.IsError {
		t.Fatalf("fsm_list: result=%+v err=%v", listResult, err)
	}
	assertMCPResultContains(t, listResult, machine.InstanceID, "mcp-machine", "idle")

	if _, err = machine.ProcessEvent("start"); err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}
	getResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "fsm_get", Arguments: map[string]any{"id": machine.InstanceID}})
	if err != nil || getResult.IsError {
		t.Fatalf("fsm_get: result=%+v err=%v", getResult, err)
	}
	assertMCPResultContains(t, getResult, machine.InstanceID, "running", `"Value":8`)
	traceResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "fsm_traces", Arguments: map[string]any{"id": machine.InstanceID, "limit": 1}})
	if err != nil || traceResult.IsError {
		t.Fatalf("fsm_traces: result=%+v err=%v", traceResult, err)
	}
	assertMCPResultContains(t, traceResult, `"event":"start"`, `"count":1`)

	server.Stop()
	if recovered := <-returned; recovered != nil {
		t.Fatalf("Start panic: %v", recovered)
	}
	if err = machine.Close(); err != nil {
		t.Fatalf("close FSM: %v", err)
	}
}

func TestManagedServerRunsWithoutConfiguredSubservices(t *testing.T) {
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)
	server := fsmserver.MustNewServer(config.Config{MCP: config.MCP{Endpoint: "/healthz"}})
	if server.Store() != nil || server.VisualizationServer() != nil || server.MCPServer() != nil {
		t.Fatalf("unexpected subservice: store=%v visualization=%v mcp=%v", server.Store(), server.VisualizationServer(), server.MCPServer())
	}
	machine := fsm.NewFSM("standalone", "idle", &testPayload{})
	machine.SetTransitions(fsm.NewTransitionItem[*testPayload]("start", "idle", "running", fsm.Handler[*testPayload]{}))
	if err := server.Register(machine); err != nil {
		t.Fatalf("register without subservices: %v", err)
	}
	if err := server.RegisterWithKey("standalone/one", machine); !errors.Is(err, fsmserver.ErrStoreNotConfigured) {
		t.Fatalf("RegisterWithKey error = %v, want ErrStoreNotConfigured", err)
	}
	if err := machine.Save(context.Background()); !errors.Is(err, fsm.ErrStoreNotBound) {
		t.Fatalf("Save error = %v, want ErrStoreNotBound", err)
	}
	returned := startManagedServerForTest(t, server)
	if server.VisualizationURL() != "" || server.MCPURL() != "" {
		t.Fatalf("unexpected URL: visualization=%q mcp=%q", server.VisualizationURL(), server.MCPURL())
	}
	server.Stop()
	if recovered := <-returned; recovered != nil {
		t.Fatalf("Start panic: %v", recovered)
	}
	if _, err := os.Stat(filepath.Join(workingDirectory, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unconfigured Store created data directory: %v", err)
	}
	if err := machine.Close(); err != nil {
		t.Fatalf("close FSM: %v", err)
	}
}

func TestManagedServerRestoresRegisteredFSMFromSharedStore(t *testing.T) {
	directory := t.TempDir()
	cfg := config.Config{
		Store: config.Store{JSONL: config.JSONL{Directory: directory, Filename: "restore.jsonl"}},
	}
	firstServer := fsmserver.MustNewServer(cfg)
	firstPayload := &testPayload{Value: 10}
	first := fsm.NewFSM("recoverable", "idle", firstPayload)
	first.SetTransitions(fsm.NewTransitionItem("start", "idle", "running", fsm.Handler[*testPayload]{
		Handler: func(machine *fsm.FSM[*testPayload], _ ...any) error {
			machine.Payload.Value = 42
			return nil
		},
	}))
	if err := firstServer.RegisterWithKey("players/player-1", first); err != nil {
		t.Fatalf("register first FSM: %v", err)
	}
	if _, err := first.ProcessEvent("start"); err != nil {
		t.Fatalf("process persisted event: %v", err)
	}
	firstServer.Stop()
	if err := first.Close(); err != nil {
		t.Fatalf("close first FSM: %v", err)
	}

	secondServer := fsmserver.MustNewServer(cfg)
	secondPayload := &testPayload{}
	second := fsm.NewFSM("recoverable", "idle", secondPayload)
	second.SetTransitions(fsm.NewTransitionItem[*testPayload]("start", "idle", "running", fsm.Handler[*testPayload]{}))
	if err := secondServer.RegisterWithKey("players/player-1", second); err != nil {
		t.Fatalf("register restored FSM: %v", err)
	}
	if got := second.GetCurrentState(); got != "running" {
		t.Fatalf("restored state = %q, want running", got)
	}
	if second.Payload != secondPayload || second.Payload.Value != 42 {
		t.Fatalf("restored payload = %+v, pointer preserved = %v", second.Payload, second.Payload == secondPayload)
	}
	secondServer.Stop()
	if err := second.Close(); err != nil {
		t.Fatalf("close restored FSM: %v", err)
	}
}

func startManagedServerForTest(t *testing.T, server *fsmserver.Server) <-chan any {
	t.Helper()
	returned := make(chan any, 1)
	go func() {
		var recovered any
		defer func() { returned <- recovered }()
		defer func() { recovered = recover() }()
		server.Start()
	}()
	select {
	case <-server.Started():
	case <-time.After(5 * time.Second):
		t.Fatal("server Start timeout")
	}
	return returned
}

func assertMCPResultContains(t *testing.T, result *mcpsdk.CallToolResult, values ...string) {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal MCP result: %v", err)
	}
	for _, value := range values {
		if !strings.Contains(string(encoded), value) {
			t.Fatalf("MCP result %s does not contain %q", encoded, value)
		}
	}
}
