package main

import (
	"context"
	"fmt"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/swkings/fsm"
	"github.com/swkings/fsm/visualization"
)

type doorPayload struct {
	Location  string `json:"location"`
	OpenCount int    `json:"openCount"`
}

// externalWorker 不依赖 fsm.FSM，只需直接实现 visualization.Client。
type externalWorker struct {
	mu       sync.RWMutex
	state    string
	progress int
}

func (worker *externalWorker) VisualizationConfig() visualization.ClientConfig {
	return visualization.ClientConfig{
		ID:           "external-worker",
		Name:         "worker",
		DisplayName:  "Third-party Worker",
		Description:  "A third-party FSM collected by the server",
		Path:         "production/workers/external",
		Mode:         visualization.ModePull,
		PullInterval: time.Second,
	}
}

func (worker *externalWorker) VisualizationSnapshot(context.Context) (visualization.Snapshot, error) {
	worker.mu.RLock()
	defer worker.mu.RUnlock()
	return visualization.Snapshot{
		CurrentState: worker.state,
		States:       []string{"idle", "running", "done"},
		Transitions: []visualization.TransitionSnapshot{
			{Event: "start", FromState: "idle", ToState: "running"},
			{Event: "finish", FromState: "running", ToState: "done"},
		},
	}, nil
}

func main() {
	server := visualization.NewServerWithConfig(visualization.ServerConfig{
		Address:    "127.0.0.1:0",
		TraceLimit: 50,
	})
	if err := server.Start(); err != nil {
		panic(err)
	}
	defer server.Close(context.Background())

	doorData := &doorPayload{Location: "warehouse"}
	door := fsm.NewFSM[doorPayload]("door", "closed", doorData)
	defer door.Close()
	door.SetTransitions(
		fsm.NewTransitionItem("open", "closed", "opened", fsm.Handler[doorPayload]{Handler: func(machine *fsm.FSM[doorPayload]) error {
			machine.Payload.OpenCount++
			return nil
		}}),
		fsm.NewTransitionItem[doorPayload]("close", "opened", "closed", fsm.Handler[doorPayload]{}),
	)
	door.SetVisualizationConfig(visualization.ClientConfig{
		ID:          "warehouse-door",
		DisplayName: "Warehouse Door",
		Path:        "production/warehouse/doors",
	})
	doorRegistration, err := server.Register(context.Background(), door)
	if err != nil {
		panic(err)
	}
	defer doorRegistration.Unregister()

	worker := &externalWorker{state: "idle"}
	workerRegistration, err := server.Register(context.Background(), worker)
	if err != nil {
		panic(err)
	}
	defer workerRegistration.Unregister()

	fmt.Printf("FSM visualization server: %s\n", server.URL())
	fmt.Println("Press Ctrl+C to stop")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go generateEvents(ctx, door, worker)
	<-ctx.Done()
}

func generateEvents(ctx context.Context, door *fsm.FSM[doorPayload], worker *externalWorker) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	doorEvent := fsm.FSMEvent("open")
	for {
		select {
		case <-ticker.C:
			_, _ = door.ProcessEvent(doorEvent)
			if doorEvent == "open" {
				doorEvent = "close"
			} else {
				doorEvent = "open"
			}
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
