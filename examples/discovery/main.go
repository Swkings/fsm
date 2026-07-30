package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/swkings/fsm"
	"github.com/swkings/fsm/visualization"
)

type workerPayload struct {
	Progress int `json:"progress"`
}

func (payload *workerPayload) JSON() ([]byte, error) { return json.Marshal(payload) }
func (payload *workerPayload) DisplayName() string   { return "Discovered Worker" }
func (payload *workerPayload) Category() string      { return "workers" }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	clientServer := visualization.NewServer(visualization.VisualizationConfig{
		ServerAddress: "127.0.0.1:5009",
		TraceLimit:    50,
	})
	if err := clientServer.Start(); err != nil {
		panic(err)
	}
	defer func() { _ = visualization.ShutdownClientServer(context.Background()) }()

	worker := newWorkerFSM()
	defer func() { _ = worker.Close() }()
	registration, err := visualization.RegisterClient(ctx, worker)
	if err != nil {
		panic(err)
	}
	defer registration.Unregister()

	registryURL, closeRegistry, err := serveRegistry(clientServer.URL())
	if err != nil {
		panic(err)
	}
	defer closeRegistry()

	aggregate := visualization.NewServerWithConfig(visualization.ServerConfig{
		Address: "127.0.0.1:5005",
	})
	if err := aggregate.Start(); err != nil {
		panic(err)
	}
	defer func() { _ = aggregate.Close(context.Background()) }()
	discovery, err := aggregate.RegisterClientServerDiscovery(ctx, &visualization.HTTPServiceDiscovery{
		Endpoint: registryURL,
	}, visualization.DiscoveryConfig{
		ID: "local-clients", Name: "Local Clients", PathPrefix: "examples/discovered",
		RefreshInterval: 2 * time.Second, PullInterval: 500 * time.Millisecond,
	})
	if err != nil {
		panic(err)
	}
	defer discovery.Unregister()

	fmt.Printf("client visualization server: %s\n", clientServer.URL())
	fmt.Printf("aggregate visualization server: %s\n", aggregate.URL())
	fmt.Printf("client server registry: %s\n", registryURL)
	fmt.Println("Press Ctrl+C to stop")
	go generateEvents(ctx, worker)
	<-ctx.Done()
}

func newWorkerFSM() *fsm.FSM[*workerPayload] {
	worker := fsm.NewFSM("worker", fsm.FSMState("idle"), &workerPayload{})
	worker.SetTransitions(
		fsm.NewTransitionItem("start", "idle", "running", fsm.Handler[*workerPayload]{
			Handler: func(machine *fsm.FSM[*workerPayload], _ ...any) error {
				machine.Payload.Progress = 50
				return nil
			},
		}),
		fsm.NewTransitionItem("finish", "running", "done", fsm.Handler[*workerPayload]{
			Handler: func(machine *fsm.FSM[*workerPayload], _ ...any) error {
				machine.Payload.Progress = 100
				return nil
			},
		}),
	)
	return worker
}

func serveRegistry(clientServerURL string) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:5010")
	if err != nil {
		return "", nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/visualization-servers", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"services": []map[string]any{{
				"key": "local-client", "endpoint": clientServerURL, "path": "client-a",
			}},
		})
	})
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpServer.Serve(listener) }()
	closeServer := func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownContext)
	}
	return "http://" + listener.Addr().String() + "/visualization-servers", closeServer, nil
}

func generateEvents(ctx context.Context, worker *fsm.FSM[*workerPayload]) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	events := []fsm.FSMEvent{"start", "finish"}
	for index := 0; index < len(events); index++ {
		select {
		case <-ticker.C:
			_, _ = worker.ProcessEvent(events[index])
		case <-ctx.Done():
			return
		}
	}
}
