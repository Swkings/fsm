package visualization

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type remoteTestClient struct {
	mu              sync.Mutex
	definition      Definition
	data            RealtimeData
	definitionCalls int
	realtimeCalls   int
}

func (client *remoteTestClient) VisualizationDefinition() Definition {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.definitionCalls++

	return cloneDefinition(client.definition)
}

func (client *remoteTestClient) VisualizationSnapshot(context.Context) (RealtimeData, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.realtimeCalls++

	return cloneRealtimeData(client.data), nil
}

func (client *remoteTestClient) updateState(state string) {
	client.mu.Lock()
	client.data.CurrentState = state
	client.data.UpdatedAt = time.Now()
	client.mu.Unlock()
}

func (client *remoteTestClient) calls() (int, int) {
	client.mu.Lock()
	defer client.mu.Unlock()

	return client.definitionCalls, client.realtimeCalls
}

type discoveryFixture struct {
	mu       sync.Mutex
	services []httpDiscoveredService
}

func (fixture *discoveryFixture) handler(writer http.ResponseWriter, request *http.Request) {
	fixture.mu.Lock()
	services := append([]httpDiscoveredService(nil), fixture.services...)
	fixture.mu.Unlock()
	writeJSON(writer, http.StatusOK, httpDiscoveryResponse{Services: services})
}

func (fixture *discoveryFixture) set(services ...httpDiscoveredService) {
	fixture.mu.Lock()
	fixture.services = append([]httpDiscoveredService(nil), services...)
	fixture.mu.Unlock()
}

func TestHTTPServiceDiscoveryAutomaticallyRegistersPullsAndRemovesFSM(t *testing.T) {
	client := &remoteTestClient{
		definition: Definition{
			ID: "remote-door", Name: "door", Description: "remote test",
			States: []string{"closed", "opened"}, Events: []string{"open", "close"},
			Transitions: []TransitionSnapshot{
				{FromState: "closed", Event: "open", ToState: "opened"},
				{FromState: "opened", Event: "close", ToState: "closed"},
			},
		},
		data: RealtimeData{CurrentState: "closed", Payload: json.RawMessage(`{"source":"remote"}`)},
	}
	clientServer := httptest.NewServer(NewClientHandler(client))
	defer clientServer.Close()

	fixture := &discoveryFixture{}
	discoveryServer := httptest.NewServer(http.HandlerFunc(fixture.handler))
	defer discoveryServer.Close()
	fixture.set(httpDiscoveredService{
		Key: "door-service", Endpoint: clientServer.URL,
		DisplayName: "Remote Door", Path: "warehouse/doors",
	})

	server := NewServer()
	defer func() { _ = server.Close(context.Background()) }()
	registration, err := server.RegisterDiscovery(context.Background(), &HTTPServiceDiscovery{
		Endpoint: discoveryServer.URL,
	}, DiscoveryConfig{
		ID: "test-discovery", Name: "Test Discovery", PathPrefix: "production",
		RefreshInterval: 10 * time.Millisecond, PullInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool {
		snapshot, ok := server.Snapshot("remote-door")

		return ok && snapshot.CurrentState == "closed" && snapshot.Path == "production/warehouse/doors"
	})
	definitionCalls, realtimeCalls := client.calls()
	if definitionCalls != 1 || realtimeCalls < 1 {
		t.Fatalf("remote calls = definition:%d realtime:%d", definitionCalls, realtimeCalls)
	}

	client.updateState("opened")
	waitFor(t, func() bool {
		snapshot, ok := server.Snapshot("remote-door")

		return ok && snapshot.CurrentState == "opened" &&
			len(snapshot.AvailableTransitions) == 1 &&
			snapshot.AvailableTransitions[0].Event == "close"
	})
	definitionCalls, _ = client.calls()
	if definitionCalls != 1 {
		t.Fatalf("definition was fetched during pull: %d", definitionCalls)
	}

	fixture.set()
	waitFor(t, func() bool {
		_, ok := server.Snapshot("remote-door")

		return !ok
	})
	if !registration.Unregister() {
		t.Fatal("discovery unregister returned false")
	}
	select {
	case <-registration.Done():
	case <-time.After(time.Second):
		t.Fatal("discovery registration did not stop")
	}
}

func TestDiscoveryConfigurationHTTPAPI(t *testing.T) {
	client := &remoteTestClient{
		definition: Definition{ID: "api-worker", Name: "worker", States: []string{"idle"}},
		data:       RealtimeData{CurrentState: "idle"},
	}
	clientVisualizationServer := NewServer()
	defer func() { _ = clientVisualizationServer.Close(context.Background()) }()
	if _, err := clientVisualizationServer.Register(context.Background(), client, RegistrationConfig{
		Mode: ModePull, PullInterval: time.Hour,
	}); err != nil {
		t.Fatal(err)
	}
	clientServer := httptest.NewServer(clientVisualizationServer.Handler())
	defer clientServer.Close()
	fixture := &discoveryFixture{}
	fixture.set(httpDiscoveredService{Key: "worker", Endpoint: clientServer.URL, Path: "workers"})
	discoveryServer := httptest.NewServer(http.HandlerFunc(fixture.handler))
	defer discoveryServer.Close()

	server := NewServer()
	defer func() { _ = server.Close(context.Background()) }()
	webServer := httptest.NewServer(server.Handler())
	defer webServer.Close()

	body, _ := json.Marshal(discoveryRequest{
		ID: "web-discovery", Name: "Web Discovery", Endpoint: discoveryServer.URL,
		PathPrefix: "external", RefreshIntervalSeconds: .02, PullIntervalSeconds: .01,
	})
	response, err := http.Post(webServer.URL+"/api/discoveries", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("create discovery status=%d body=%s", response.StatusCode, data)
	}
	_ = response.Body.Close()
	waitFor(t, func() bool {
		snapshot, ok := server.Snapshot("remote:worker:api-worker")

		return ok && snapshot.Path == "external/workers"
	})

	response, err = http.Get(webServer.URL + "/api/discoveries")
	if err != nil {
		t.Fatal(err)
	}
	var views []discoveryView
	if err := json.NewDecoder(response.Body).Decode(&views); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if len(views) != 1 || views[0].ID != "web-discovery" || views[0].Registered != 1 {
		t.Fatalf("discovery views = %+v", views)
	}

	response, err = http.Post(webServer.URL+"/api/discoveries/web-discovery/refresh", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("refresh discovery status=%d body=%s", response.StatusCode, data)
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodDelete, webServer.URL+"/api/discoveries/web-discovery", nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete discovery status=%d", response.StatusCode)
	}
	_ = response.Body.Close()
	waitFor(t, func() bool {
		_, ok := server.Snapshot("remote:worker:api-worker")

		return !ok
	})
}
