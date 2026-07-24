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

// externalWorker 演示一个不依赖 fsm 包实现的第三方状态机。
// 它通过实现 visualization.Registrable 决定以 pull 模式接入 Server。
type externalWorker struct {
	mu       sync.RWMutex
	state    string
	progress int
}

func (worker *externalWorker) RegisterVisualization(
	ctx context.Context,
	registrar visualization.Registrar,
) (visualization.Registration, error) {
	return registrar.RegisterPull(ctx, worker,
		visualization.WithRegistrationID("external-worker"),
		visualization.WithDisplayName("Third-party Worker"),
		visualization.WithDescription("A third-party FSM collected by the server"),
		visualization.WithPath("production/workers/external"),
		visualization.WithPullInterval(time.Second),
	)
}

func (worker *externalWorker) Descriptor() visualization.Descriptor {
	return visualization.Descriptor{ID: "external-worker", Name: "worker"}
}

func (worker *externalWorker) Collect(context.Context) (visualization.Snapshot, error) {
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
	server := visualization.NewServer(
		visualization.WithAddress("0.0.0.0:5005"),
		visualization.WithTraceLimit(50),
	)
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
	registration, err := door.RegisterVisualization(context.Background(), server,
		visualization.WithRegistrationID("warehouse-door"),
		visualization.WithDisplayName("Warehouse Door"),
		visualization.WithPath("production/warehouse/doors"),
	)
	if err != nil {
		panic(err)
	}
	defer registration.Unregister()

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
