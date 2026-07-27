package visualization

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type thirdPartyFSM struct {
	mu              sync.Mutex
	definitionCalls int
	snapshotCalls   int
	collected       int
	fail            bool
}

func (machine *thirdPartyFSM) VisualizationDefinition() Definition {
	machine.mu.Lock()
	machine.definitionCalls++
	machine.mu.Unlock()
	return Definition{
		ID:          "third-party-order",
		Name:        "order",
		Description: "external fsm implementation",
		States:      []string{"pending", "running", "done"},
		Events:      []string{"start", "finish"},
		Transitions: []TransitionSnapshot{
			{FromState: "pending", Event: "start", ToState: "running"},
			{FromState: "running", Event: "finish", ToState: "done"},
		},
	}
}

func (machine *thirdPartyFSM) VisualizationSnapshot(context.Context) (RealtimeData, error) {
	machine.mu.Lock()
	defer machine.mu.Unlock()
	machine.collected++
	if machine.fail {
		return RealtimeData{}, errors.New("collector unavailable")
	}
	state := "pending"
	if machine.collected > 1 {
		state = "running"
	}
	payload, _ := json.Marshal(map[string]any{"collected": machine.collected})
	return RealtimeData{CurrentState: state, Payload: payload}, nil
}

func (machine *thirdPartyFSM) counts() (definitionCalls, collected int) {
	machine.mu.Lock()
	defer machine.mu.Unlock()
	return machine.definitionCalls, machine.collected
}

type testPushClient struct {
	mu              sync.Mutex
	definition      Definition
	data            RealtimeData
	definitionCalls int
	snapshotCalls   int
	publish         PublishFunc
	unsubscribed    bool
}

func (client *testPushClient) VisualizationDefinition() Definition {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.definitionCalls++
	return cloneDefinition(client.definition)
}

func (client *testPushClient) VisualizationSnapshot(context.Context) (RealtimeData, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.snapshotCalls++
	return cloneRealtimeData(client.data), nil
}

func (client *testPushClient) SubscribeVisualization(publish PublishFunc) func() {
	client.mu.Lock()
	client.publish = publish
	client.mu.Unlock()
	return func() {
		client.mu.Lock()
		client.publish = nil
		client.unsubscribed = true
		client.mu.Unlock()
	}
}

func (client *testPushClient) update(data RealtimeData, traces ...TraceSnapshot) error {
	client.mu.Lock()
	client.data = data
	publish := client.publish
	client.mu.Unlock()
	if publish == nil {
		return errors.New("push client is not subscribed")
	}
	return publish(data, traces...)
}

func TestServerRegistersPullClientAndReadsDefinitionOnce(t *testing.T) {
	server := NewServerWithConfig(ServerConfig{TraceLimit: 2})
	defer server.Close(context.Background())

	machine := &thirdPartyFSM{}
	registration, err := server.Register(context.Background(), machine, RegistrationConfig{
		DisplayName:  "Third-party Order",
		Path:         "/production / orders/",
		Mode:         ModePull,
		PullInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if registration.Mode() != ModePull || registration.ID() != "third-party-order" {
		t.Fatalf("registration = id:%q mode:%q", registration.ID(), registration.Mode())
	}
	waitFor(t, func() bool {
		_, collected := machine.counts()
		return collected >= 2
	})

	definitionCalls, _ := machine.counts()
	if definitionCalls != 1 {
		t.Fatalf("VisualizationDefinition calls = %d, want 1", definitionCalls)
	}
	snapshot, exists := server.Snapshot(registration.ID())
	if !exists {
		t.Fatal("pull snapshot not found")
	}
	if snapshot.Path != "production/orders" || snapshot.Mode != ModePull {
		t.Fatalf("pull metadata = %+v", snapshot)
	}
	if snapshot.CurrentState != "running" || len(snapshot.States) != 3 || len(snapshot.Events) != 2 {
		t.Fatalf("pull definition/runtime merge = %+v", snapshot)
	}
	if len(snapshot.AvailableTransitions) != 1 ||
		snapshot.AvailableTransitions[0].Event != "finish" ||
		snapshot.AvailableTransitions[0].ToState != "done" {
		t.Fatalf("available transitions = %+v", snapshot.AvailableTransitions)
	}

	machine.mu.Lock()
	machine.fail = true
	machine.mu.Unlock()
	waitFor(t, func() bool {
		current, _ := server.Snapshot(registration.ID())
		return strings.Contains(current.CollectionError, "collector unavailable")
	})
	if !registration.Unregister() {
		t.Fatal("Unregister() = false")
	}
	select {
	case <-registration.Done():
	case <-time.After(time.Second):
		t.Fatal("registration Done was not closed")
	}
}

func TestServerPushOnlyUpdatesRealtimeData(t *testing.T) {
	server := NewServerWithConfig(ServerConfig{TraceLimit: 2})
	defer server.Close(context.Background())

	client := &testPushClient{
		definition: Definition{
			ID:          "worker-7",
			Name:        "worker",
			States:      []string{"idle", "running"},
			Events:      []string{"run"},
			Transitions: []TransitionSnapshot{{FromState: "idle", Event: "run", ToState: "running"}},
		},
		data: RealtimeData{CurrentState: "idle", Payload: json.RawMessage(`{"job":7}`)},
	}
	registration, err := server.Register(context.Background(), client, RegistrationConfig{
		DisplayName: "Worker 7",
		Path:        "region-a / workers",
		Mode:        ModePush,
	})
	if err != nil {
		t.Fatal(err)
	}

	client.mu.Lock()
	client.definition.States = []string{"corrupted"}
	client.definition.Transitions = nil
	client.mu.Unlock()
	for index := 0; index < 3; index++ {
		trace := TraceSnapshot{
			TransitionID: fmt.Sprintf("trace-%d", index),
			FromState:    "idle",
			Event:        "run",
			ToState:      "running",
			FinalState:   "running",
			Success:      true,
		}
		if err := client.update(RealtimeData{
			CurrentState: "running",
			Payload:      json.RawMessage(`{"job":7}`),
		}, trace); err != nil {
			t.Fatal(err)
		}
	}
	stored, exists := server.Snapshot(registration.ID())
	if !exists {
		t.Fatal("push snapshot not found")
	}
	if stored.Mode != ModePush || stored.Path != "region-a/workers" || stored.DisplayName != "Worker 7" {
		t.Fatalf("push metadata = %+v", stored)
	}
	if len(stored.States) != 2 || len(stored.Transitions) != 1 || stored.Transitions[0].Event != "run" {
		t.Fatalf("registered definition was changed: %+v", stored)
	}
	if len(stored.Traces) != 2 || stored.Traces[0].TransitionID != "trace-1" ||
		stored.Traces[1].TransitionID != "trace-2" {
		t.Fatalf("trace limit/order = %+v", stored.Traces)
	}
	client.mu.Lock()
	definitionCalls := client.definitionCalls
	snapshotCalls := client.snapshotCalls
	client.mu.Unlock()
	if snapshotCalls != 1 {
		t.Fatalf("VisualizationSnapshot calls = %d after pushes, want initial call only", snapshotCalls)
	}
	if definitionCalls != 1 {
		t.Fatalf("VisualizationDefinition calls = %d, want 1", definitionCalls)
	}

	registration.Unregister()
	client.mu.Lock()
	unsubscribed := client.unsubscribed
	client.mu.Unlock()
	if !unsubscribed {
		t.Fatal("push client unsubscribe was not called")
	}
}

func TestServerHTTPAPIWebSSEAndPushGateway(t *testing.T) {
	server := NewServer()
	defer server.Close(context.Background())
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response, err := http.Get(httpServer.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	required := []string{
		"FSM Observatory",
		"EventSource",
		"buildFolderTree",
		"folder-node",
		"themeToggle",
		"exportStateGraph",
		"graph-stage",
		"setupGraphPan",
		"node-tooltip",
		"showNodeTooltip",
		"floating-panel",
		"initializeFloatingPanels",
		"sidebarToggle",
		"applySidebarCollapsed",
		"body.sidebar-collapsed #sidebarResize { visibility: hidden; pointer-events: none; }",
		"opacityToggle",
		"discoveryToggle",
		"discoveryForm",
		"settings-modal",
		"settings-nav-item",
		"discovery-form-footer",
		"打开设置",
		"/api/discoveries",
		"applyOverlayTransparency",
		"--panel-alpha: 15%",
		"#faf9f5",
		"#cc785c",
	}
	pageText := string(page)
	for _, marker := range required {
		if !strings.Contains(pageText, marker) {
			t.Fatalf("web page missing %q", marker)
		}
	}
	if strings.Index(pageText, "id=\"discoveryToggle\"") < strings.Index(pageText, "id=\"themeToggle\"") {
		t.Fatal("settings toggle must be the rightmost header action")
	}

	runtimeBody := []byte(`{"currentState":"ready","payload":{"source":"http"}}`)
	response, err = http.Post(httpServer.URL+"/api/push/remote-1", "application/json", bytes.NewReader(runtimeBody))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("push before registration status = %d", response.StatusCode)
	}
	_ = response.Body.Close()

	streamContext, cancelStream := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelStream()
	request, _ := http.NewRequestWithContext(streamContext, http.MethodGet, httpServer.URL+"/api/events", nil)
	streamResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer streamResponse.Body.Close()
	events := make(chan streamEvent, 4)
	go readStreamEvents(streamResponse.Body, events)
	if event := waitStreamEvent(t, events); event.Type != "connected" {
		t.Fatalf("first SSE event = %+v", event)
	}

	definitionBody := []byte(`{"id":"remote-1","name":"remote","displayName":"Remote FSM","path":"external/team-a","states":["ready","busy"],"events":["run"],"transitions":[{"fromState":"ready","event":"run","toState":"busy"}]}`)
	response, err = http.Post(httpServer.URL+"/api/fsms", "application/json", bytes.NewReader(definitionBody))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("registration status=%d body=%s", response.StatusCode, data)
	}
	_ = response.Body.Close()

	invalidRuntimeBody := []byte(`{"currentState":"ready","states":["forged"]}`)
	response, err = http.Post(httpServer.URL+"/api/push/remote-1", "application/json", bytes.NewReader(invalidRuntimeBody))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("push containing static definition status = %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, err = http.Post(httpServer.URL+"/api/push/remote-1", "application/json", bytes.NewReader(runtimeBody))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("push status=%d body=%s", response.StatusCode, data)
	}
	_ = response.Body.Close()

	for {
		event := waitStreamEvent(t, events)
		if event.Type == "updated" && event.FSMID == "remote-1" {
			break
		}
	}
	response, err = http.Get(httpServer.URL + "/api/fsms/remote-1")
	if err != nil {
		t.Fatal(err)
	}
	var detail Snapshot
	if err := json.NewDecoder(response.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if detail.ID != "remote-1" || detail.Path != "external/team-a" ||
		detail.CurrentState != "ready" || detail.Mode != ModePush ||
		len(detail.Transitions) != 1 || len(detail.AvailableTransitions) != 1 {
		t.Fatalf("HTTP push detail = %+v", detail)
	}
}

type configuredClient struct {
	definition Definition
	data       RealtimeData
}

func (client *configuredClient) VisualizationDefinition() Definition { return client.definition }

func (client *configuredClient) VisualizationSnapshot(context.Context) (RealtimeData, error) {
	return client.data, nil
}

func TestServerRegistrationValidationAndModeIsolation(t *testing.T) {
	server := NewServer()
	defer server.Close(context.Background())

	var nilClient *configuredClient
	if _, err := server.Register(
		context.Background(),
		nilClient,
		RegistrationConfig{},
	); !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("typed nil client error = %v", err)
	}
	if _, err := server.Register(
		context.Background(),
		&configuredClient{definition: Definition{ID: "invalid/path"}},
		RegistrationConfig{},
	); !errors.Is(err, ErrInvalidRegistrationID) {
		t.Fatalf("invalid id error = %v", err)
	}
	if _, err := server.Register(
		context.Background(),
		&configuredClient{definition: Definition{ID: "invalid-path"}},
		RegistrationConfig{Path: "a//b"},
	); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("invalid path error = %v", err)
	}
	first := &configuredClient{definition: Definition{ID: "same"}}
	if _, err := server.Register(context.Background(), first, RegistrationConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Register(
		context.Background(),
		&configuredClient{definition: Definition{ID: "same"}},
		RegistrationConfig{},
	); !errors.Is(err, ErrRegistrationIDRepeated) {
		t.Fatalf("duplicate id error = %v", err)
	}
	pullRegistration, err := server.Register(
		context.Background(),
		&configuredClient{
			definition: Definition{ID: "pull-only"},
			data:       RealtimeData{CurrentState: "ok"},
		},
		RegistrationConfig{Mode: ModePull, PullInterval: time.Hour},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Push(context.Background(), pullRegistration.ID(), RealtimeData{}); !errors.Is(err, ErrRegistrationMode) {
		t.Fatalf("push to pull registration error = %v", err)
	}
	if err := server.Push(context.Background(), "missing", RealtimeData{}); !errors.Is(err, ErrRegistrationNotFound) {
		t.Fatalf("push missing registration error = %v", err)
	}
	if _, err := server.Register(
		context.Background(),
		&configuredClient{definition: Definition{ID: "wrong-push"}},
		RegistrationConfig{Mode: ModePush},
	); !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("push mode without PushClient error = %v", err)
	}
}

func TestServerStartAndClose(t *testing.T) {
	server := NewServerWithConfig(ServerConfig{Address: "127.0.0.1:0"})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	if server.URL() == "" {
		t.Fatal("URL() is empty after Start")
	}
	response, err := http.Get(server.URL() + "/api/fsms")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("api status = %d", response.StatusCode)
	}
	if err := server.Start(); !errors.Is(err, ErrServerStarted) {
		t.Fatalf("second Start error = %v", err)
	}

	client := &testPushClient{
		definition: Definition{ID: "close-me"},
		data:       RealtimeData{CurrentState: "idle"},
	}
	registration, err := server.Register(context.Background(), client, RegistrationConfig{Mode: ModePush})
	if err != nil {
		t.Fatal(err)
	}
	closeContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(closeContext); err != nil {
		t.Fatal(err)
	}
	select {
	case <-registration.Done():
	case <-time.After(time.Second):
		t.Fatal("registration Done was not closed")
	}
	if err := server.Start(); !errors.Is(err, ErrServerClosed) {
		t.Fatalf("Start after Close error = %v", err)
	}
}

type panicClient struct{}

func (*panicClient) VisualizationDefinition() Definition {
	return Definition{ID: "panic-source"}
}

func (*panicClient) VisualizationSnapshot(context.Context) (RealtimeData, error) {
	panic("third-party failure")
}

func TestRegistrationContextCancellationAndSnapshotPanicIsolation(t *testing.T) {
	server := NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	registration, err := server.Register(ctx, &panicClient{}, RegistrationConfig{
		Mode: ModePull, PullInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("register panic client: %v", err)
	}
	snapshot, ok := server.Snapshot("panic-source")
	if !ok || !strings.Contains(snapshot.CollectionError, "third-party failure") {
		t.Fatalf("snapshot panic was not isolated: %+v, found=%v", snapshot, ok)
	}

	cancel()
	select {
	case <-registration.Done():
	case <-time.After(time.Second):
		t.Fatal("registration did not stop after context cancellation")
	}
	if _, ok := server.Snapshot("panic-source"); ok {
		t.Fatal("registration still exists after context cancellation")
	}
}

func readStreamEvents(reader io.Reader, events chan<- streamEvent) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event streamEvent
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil {
			events <- event
		}
	}
}

func waitStreamEvent(t *testing.T, events <-chan streamEvent) streamEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for visualization SSE event")
		return streamEvent{}
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}
