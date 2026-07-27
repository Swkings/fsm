package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/swkings/fsm/visualization"
)

type remoteWorker struct {
	mu       sync.RWMutex
	state    string
	progress int
}

func (worker *remoteWorker) VisualizationDefinition() visualization.Definition {
	return visualization.Definition{
		ID: "remote-worker", Name: "worker", Description: "discovered remote FSM",
		States: []string{"idle", "running", "done"}, Events: []string{"start", "finish"},
		Transitions: []visualization.TransitionSnapshot{
			{FromState: "idle", Event: "start", ToState: "running"},
			{FromState: "running", Event: "finish", ToState: "done"},
		},
	}
}

func (worker *remoteWorker) VisualizationSnapshot(context.Context) (visualization.RealtimeData, error) {
	worker.mu.RLock()
	defer worker.mu.RUnlock()
	payload, _ := json.Marshal(map[string]int{"progress": worker.progress})
	return visualization.RealtimeData{CurrentState: worker.state, Payload: payload, UpdatedAt: time.Now()}, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	worker := &remoteWorker{state: "idle"}
	discoveryURL, closeRemote, err := serveRemoteClient(worker)
	if err != nil {
		panic(err)
	}
	defer closeRemote()

	server := visualization.NewServer()
	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Close(context.Background())

	discovery, err := server.RegisterDiscovery(ctx, &visualization.HTTPServiceDiscovery{
		Endpoint: discoveryURL,
	}, visualization.DiscoveryConfig{
		ID: "local-services", Name: "Local Services", PathPrefix: "examples/discovered",
		RefreshInterval: 2 * time.Second, PullInterval: 500 * time.Millisecond,
	})
	if err != nil {
		panic(err)
	}
	defer discovery.Unregister()

	fmt.Printf("FSM visualization server: %s\n", server.URL())
	fmt.Printf("service discovery endpoint: %s\n", discoveryURL)
	fmt.Println("Press Ctrl+C to stop")
	go updateWorker(ctx, worker)
	<-ctx.Done()
}

func serveRemoteClient(worker *remoteWorker) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:5009")
	if err != nil {
		return "", nil, err
	}
	baseURL := "http://" + listener.Addr().String()
	mux := http.NewServeMux()
	mux.Handle("/fsm/worker/", http.StripPrefix("/fsm/worker", visualization.NewClientHandler(worker)))
	mux.HandleFunc("/services", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"services": []map[string]any{{
				"key": "worker-1", "endpoint": baseURL + "/fsm/worker",
				"displayName": "Discovered Worker", "path": "workers",
			}},
		})
	})
	httpServer := &http.Server{Handler: mux}
	go func() { _ = httpServer.Serve(listener) }()
	closeServer := func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownContext)
	}
	return baseURL + "/services", closeServer, nil
}

func updateWorker(ctx context.Context, worker *remoteWorker) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			worker.mu.Lock()
			worker.progress++
			switch worker.progress {
			case 1:
				worker.state = "running"
			case 5:
				worker.state = "done"
			}
			worker.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}
