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
	mu        sync.Mutex
	collected int
	fail      bool
}

func (machine *thirdPartyFSM) Descriptor() Descriptor {
	return Descriptor{ID: "third-party-order", Name: "order", DisplayName: "Third-party Order", Description: "external fsm implementation"}
}

func (machine *thirdPartyFSM) Collect(context.Context) (Snapshot, error) {
	machine.mu.Lock()
	defer machine.mu.Unlock()
	machine.collected++
	if machine.fail {
		return Snapshot{}, errors.New("collector unavailable")
	}
	state := "pending"
	if machine.collected > 1 {
		state = "running"
	}
	payload, _ := json.Marshal(map[string]any{"collected": machine.collected})
	return Snapshot{
		CurrentState: state,
		Payload:      payload,
		Transitions: []TransitionSnapshot{
			{FromState: "pending", Event: "start", ToState: "running"},
			{FromState: "running", Event: "finish", ToState: "done"},
		},
	}, nil
}

func (machine *thirdPartyFSM) RegisterVisualization(ctx context.Context, registrar Registrar) (Registration, error) {
	return registrar.RegisterPull(ctx, machine, WithPath("/production / orders/"), WithPullInterval(10*time.Millisecond))
}

func (machine *thirdPartyFSM) collectCount() int {
	machine.mu.Lock()
	defer machine.mu.Unlock()
	return machine.collected
}

func TestServerRegistersThirdPartyFSMAndPullsPeriodically(t *testing.T) {
	server := NewServer(WithTraceLimit(2))
	defer server.Close(context.Background())

	machine := &thirdPartyFSM{}
	registration, err := server.Register(context.Background(), machine)
	if err != nil {
		t.Fatal(err)
	}
	if registration.Mode() != ModePull || registration.ID() != "third-party-order" {
		t.Fatalf("registration = id:%q mode:%q", registration.ID(), registration.Mode())
	}
	waitFor(t, func() bool { return machine.collectCount() >= 2 })

	snapshot, exists := server.Snapshot(registration.ID())
	if !exists {
		t.Fatal("pull snapshot not found")
	}
	if snapshot.Path != "production/orders" || snapshot.Mode != ModePull {
		t.Fatalf("pull metadata = %+v", snapshot)
	}
	if snapshot.CurrentState != "running" || len(snapshot.States) != 3 {
		t.Fatalf("pull topology = %+v", snapshot)
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

func TestServerPushModeStoresIncrementalTracesAndPath(t *testing.T) {
	server := NewServer(WithTraceLimit(2))
	defer server.Close(context.Background())

	pusher, err := server.RegisterPush(
		context.Background(),
		Descriptor{ID: "worker-7", Name: "worker"},
		WithDisplayName("Worker 7"),
		WithPath("region-a / workers"),
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{
		CurrentState: "idle",
		Payload:      json.RawMessage("{\"job\":7}"),
		Transitions:  []TransitionSnapshot{{FromState: "idle", Event: "run", ToState: "running"}},
	}
	for index := 0; index < 3; index++ {
		trace := TraceSnapshot{
			TransitionID: fmt.Sprintf("trace-%d", index),
			FromState:    "idle",
			Event:        "run",
			ToState:      "running",
			FinalState:   "running",
			Success:      true,
		}
		if err := pusher.Push(context.Background(), snapshot, trace); err != nil {
			t.Fatal(err)
		}
	}
	stored, exists := server.Snapshot(pusher.ID())
	if !exists {
		t.Fatal("push snapshot not found")
	}
	if stored.Mode != ModePush || stored.Path != "region-a/workers" || stored.DisplayName != "Worker 7" {
		t.Fatalf("push metadata = %+v", stored)
	}
	if len(stored.Traces) != 2 || stored.Traces[0].TransitionID != "trace-1" ||
		stored.Traces[1].TransitionID != "trace-2" {
		t.Fatalf("trace limit/order = %+v", stored.Traces)
	}
	if len(stored.AvailableTransitions) != 1 || stored.AvailableTransitions[0].ToState != "running" {
		t.Fatalf("push topology = %+v", stored)
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
		"#faf9f5",
		"#cc785c",
	}
	for _, marker := range required {
		if !strings.Contains(string(page), marker) {
			t.Fatalf("web page missing %q", marker)
		}
	}

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

	body := []byte("{\"name\":\"remote\",\"displayName\":\"Remote FSM\",\"path\":\"external/team-a\",\"currentState\":\"ready\",\"transitions\":[{\"fromState\":\"ready\",\"event\":\"run\",\"toState\":\"busy\"}],\"payload\":{\"source\":\"http\"}}")
	response, err = http.Post(httpServer.URL+"/api/push/remote-1", "application/json", bytes.NewReader(body))
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
		detail.CurrentState != "ready" || detail.Mode != ModePush {
		t.Fatalf("HTTP push detail = %+v", detail)
	}
}

func TestServerRegistrationValidationAndModeIsolation(t *testing.T) {
	server := NewServer()
	defer server.Close(context.Background())

	var nilSource *thirdPartyFSM
	if _, err := server.RegisterPull(context.Background(), nilSource); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("typed nil pull source error = %v", err)
	}
	var nilTarget *thirdPartyFSM
	if _, err := server.Register(context.Background(), nilTarget); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("typed nil registrable error = %v", err)
	}
	if _, err := server.RegisterPush(context.Background(), Descriptor{ID: "invalid/path"}); !errors.Is(err, ErrInvalidRegistrationID) {
		t.Fatalf("invalid id error = %v", err)
	}
	if _, err := server.RegisterPush(
		context.Background(),
		Descriptor{ID: "invalid-path"},
		WithPath("a//b"),
	); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("invalid path error = %v", err)
	}
	pusher, err := server.RegisterPush(context.Background(), Descriptor{ID: "same"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.RegisterPush(context.Background(), Descriptor{ID: "same"}); !errors.Is(err, ErrRegistrationIDRepeated) {
		t.Fatalf("duplicate id error = %v", err)
	}

	pullRegistration, err := server.RegisterPull(
		context.Background(),
		PullSourceFunc{
			Meta: Descriptor{ID: "pull-only"},
			CollectFunc: func(context.Context) (Snapshot, error) {
				return Snapshot{CurrentState: "ok"}, nil
			},
		},
		WithPullInterval(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Push(context.Background(), pullRegistration.ID(), Snapshot{}); !errors.Is(err, ErrRegistrationMode) {
		t.Fatalf("push to pull registration error = %v", err)
	}
	if err := server.Push(context.Background(), "missing", Snapshot{}); !errors.Is(err, ErrRegistrationNotFound) {
		t.Fatalf("push missing registration error = %v", err)
	}
	if err := pusher.Push(context.Background(), Snapshot{CurrentState: "ok"}); err != nil {
		t.Fatal(err)
	}
}

func TestServerStartAndClose(t *testing.T) {
	server := NewServer(WithAddress("127.0.0.1:0"))
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

	pusher, err := server.RegisterPush(context.Background(), Descriptor{ID: "close-me"})
	if err != nil {
		t.Fatal(err)
	}
	closeContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(closeContext); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pusher.Done():
	case <-time.After(time.Second):
		t.Fatal("pusher Done was not closed")
	}
	if err := server.Start(); !errors.Is(err, ErrServerClosed) {
		t.Fatalf("Start after Close error = %v", err)
	}
}

func TestRegistrationContextCancellationAndPullPanicIsolation(t *testing.T) {
	server := NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	registration, err := server.RegisterPull(ctx, PullSourceFunc{
		Meta: Descriptor{ID: "panic-source"},
		CollectFunc: func(context.Context) (Snapshot, error) {
			panic("third-party failure")
		},
	}, WithPullInterval(time.Hour))
	if err != nil {
		t.Fatalf("register panic source: %v", err)
	}
	snapshot, ok := server.Snapshot("panic-source")
	if !ok || !strings.Contains(snapshot.CollectionError, "third-party failure") {
		t.Fatalf("pull panic was not isolated: %+v, found=%v", snapshot, ok)
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
