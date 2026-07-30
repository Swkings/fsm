package visualization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func (client *testPushClient) VisualizationRegistrationConfig() RegistrationConfig {
	client.mu.Lock()
	defer client.mu.Unlock()

	return RegistrationConfig{
		DisplayName: "Registered " + client.definition.ID,
		Path:        "workers/" + client.definition.ID,
	}
}

func TestNewServerConfigRegistersAndPushesMultipleFSMs(t *testing.T) {
	_ = ShutdownClientServer(context.Background())
	t.Cleanup(func() { _ = ShutdownClientServer(context.Background()) })

	server := NewServer(VisualizationConfig{
		ServerAddress: "127.0.0.1:0",
		TraceLimit:    4,
		PathPrefix:    "production",
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	if ClientServerURL() == "" {
		t.Fatal("client server URL is empty")
	}
	if same := NewServer(VisualizationConfig{ServerAddress: "127.0.0.1:5006"}); same != server {
		t.Fatalf("NewServer returned another singleton: %p != %p", same, server)
	}

	first := newPushClient("client-worker-a", "idle")
	second := newPushClient("client-worker-b", "waiting")
	firstRegistration, err := RegisterClient(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	secondRegistration, err := RegisterClient(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if len(server.Snapshots()) != 2 {
		t.Fatalf("client server snapshots = %+v", server.Snapshots())
	}
	firstSnapshot, exists := server.Snapshot(firstRegistration.ID())
	if !exists ||
		firstSnapshot.DisplayName != "Registered client-worker-a" ||
		firstSnapshot.Path != "production/workers/client-worker-a" {
		t.Fatalf("client-provided registration metadata = %+v exists=%t", firstSnapshot, exists)
	}
	if err := first.update(RealtimeData{CurrentState: "running", Payload: json.RawMessage(`{"job":"a"}`)}); err != nil {
		t.Fatal(err)
	}
	snapshot, exists := server.Snapshot(firstRegistration.ID())
	if !exists || snapshot.CurrentState != "running" {
		t.Fatalf("pushed snapshot = %+v exists=%t", snapshot, exists)
	}

	response, err := http.Get(ClientServerURL() + "/api/fsms")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("client frontend API status = %d", response.StatusCode)
	}
	firstRegistration.Unregister()
	secondRegistration.Unregister()
}

func TestClientServerDiscoveryAggregatesMultipleServers(t *testing.T) {
	firstSource, firstHTTP, firstClient := newExportedTestServer(t, "worker", "idle", "Workers/A")
	secondSource, secondHTTP, _ := newExportedTestServer(t, "worker", "waiting", "Workers/B")
	defer func() { _ = firstSource.Close(context.Background()) }()
	defer func() { _ = secondSource.Close(context.Background()) }()
	defer firstHTTP.Close()
	defer secondHTTP.Close()

	var sourceMu sync.RWMutex
	discoveredServers := []DiscoveredService{
		{Key: "client-a", Endpoint: firstHTTP.URL, Path: "clients/a", PullInterval: 10 * time.Millisecond},
		{Key: "client-b", Endpoint: secondHTTP.URL, Path: "clients/b", PullInterval: 10 * time.Millisecond},
	}
	discovery := ServiceDiscoveryFunc(func(context.Context) ([]DiscoveredService, error) {
		sourceMu.RLock()
		defer sourceMu.RUnlock()

		return append([]DiscoveredService(nil), discoveredServers...), nil
	})

	aggregate := NewServer()
	defer func() { _ = aggregate.Close(context.Background()) }()
	registration, err := aggregate.RegisterClientServerDiscovery(context.Background(), discovery, DiscoveryConfig{
		ID: "clients", PathPrefix: "production",
		RefreshInterval: 10 * time.Millisecond, PullInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()
	waitFor(t, func() bool {
		first, firstOK := aggregate.Snapshot("remote:client-a:worker")
		second, secondOK := aggregate.Snapshot("remote:client-b:worker")

		return firstOK && secondOK && first.CurrentState == "idle" && second.CurrentState == "waiting" &&
			first.Path == "production/clients/a/Workers/A" &&
			second.Path == "production/clients/b/Workers/B"
	})

	if err := firstClient.update(RealtimeData{CurrentState: "running"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		snapshot, exists := aggregate.Snapshot("remote:client-a:worker")

		return exists && snapshot.CurrentState == "running"
	})

	sourceMu.Lock()
	discoveredServers = discoveredServers[1:]
	sourceMu.Unlock()
	waitFor(t, func() bool {
		_, firstExists := aggregate.Snapshot("remote:client-a:worker")
		_, secondExists := aggregate.Snapshot("remote:client-b:worker")

		return !firstExists && secondExists
	})
}

func newPushClient(id, state string) *testPushClient {
	return &testPushClient{
		definition: Definition{
			ID: id, Name: "worker",
			States: []string{"idle", "waiting", "running"},
			Events: []string{"run"},
			Transitions: []TransitionSnapshot{
				{FromState: state, Event: "run", ToState: "running"},
			},
		},
		data: RealtimeData{CurrentState: state},
	}
}

func newExportedTestServer(
	t *testing.T,
	id string,
	state string,
	path string,
) (*Server, *httptest.Server, *testPushClient) {
	t.Helper()
	server := NewServer()
	client := newPushClient(id, state)
	if _, err := server.Register(context.Background(), client, RegistrationConfig{
		DisplayName: id + " " + state, Path: path,
	}); err != nil {
		t.Fatal(err)
	}

	return server, httptest.NewServer(server.Handler()), client
}
