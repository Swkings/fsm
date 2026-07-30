package visualization

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDiscoveryConfigurationHTTPAPIAcceptsDirectClientServerAddress(t *testing.T) {
	client := &remoteTestClient{
		definition: Definition{
			ID: "direct-worker", Name: "worker", States: []string{"idle", "running"},
		},
		data: RealtimeData{CurrentState: "idle"},
	}
	clientVisualizationServer := NewServer()
	defer func() { _ = clientVisualizationServer.Close(context.Background()) }()
	if _, err := clientVisualizationServer.Register(context.Background(), client, RegistrationConfig{
		DisplayName: "Direct Worker", Path: "workers",
		Mode: ModePull, PullInterval: time.Hour,
	}); err != nil {
		t.Fatal(err)
	}
	clientHTTP := httptest.NewServer(clientVisualizationServer.Handler())
	defer clientHTTP.Close()

	aggregate := NewServer()
	defer func() { _ = aggregate.Close(context.Background()) }()
	aggregateHTTP := httptest.NewServer(aggregate.Handler())
	defer aggregateHTTP.Close()
	body, _ := json.Marshal(discoveryRequest{
		ID: "direct-client", Name: "Direct Client", Endpoint: clientHTTP.URL,
		PathPrefix: "external", RefreshIntervalSeconds: .02, PullIntervalSeconds: .01,
	})
	response, err := http.Post(
		aggregateHTTP.URL+"/api/discoveries",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("create direct discovery status=%d body=%s", response.StatusCode, data)
	}
	waitFor(t, func() bool {
		snapshot, exists := aggregate.Snapshot("remote:direct-client:direct-worker")

		return exists && snapshot.CurrentState == "idle" &&
			snapshot.DisplayName == "Direct Worker" &&
			snapshot.Path == "external/workers"
	})
	discoveries := aggregate.DiscoverySnapshots()
	if len(discoveries) != 1 || discoveries[0].Endpoint != clientHTTP.URL ||
		discoveries[0].Registered != 1 {
		t.Fatalf("direct discovery status = %+v", discoveries)
	}
}
