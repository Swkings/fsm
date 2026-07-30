package main

import (
	"context"
	"encoding/json"
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

func (p *doorPayload) JSON() ([]byte, error) { return json.Marshal(p) }
func (p *doorPayload) DisplayName() string   { return "Warehouse Door" }
func (p *doorPayload) Category() string      { return "warehouse/doors" }

// externalWorker 不依赖 fsm.FSM，只需直接实现 visualization.ClientI。
type externalWorker struct {
	mu       sync.RWMutex
	state    string
	progress int
}

// VisualizationDefinition 只在注册时调用一次，返回完整状态、事件和转移图。
func (worker *externalWorker) VisualizationDefinition() visualization.Definition {
	return visualization.Definition{
		ID:          "external-worker",
		Name:        "worker",
		Description: "A third-party FSM collected by the server",
		States:      []string{"idle", "running", "done"},
		Events:      []string{"start", "finish"},
		Transitions: []visualization.TransitionSnapshot{
			{Event: "start", FromState: "idle", ToState: "running"},
			{Event: "finish", FromState: "running", ToState: "done"},
		},
	}
}

// VisualizationSnapshot 在后续 pull 中只返回实时数据。
func (worker *externalWorker) VisualizationSnapshot(context.Context) (visualization.RealtimeData, error) {
	worker.mu.RLock()
	defer worker.mu.RUnlock()
	return visualization.RealtimeData{CurrentState: worker.state}, nil
}

func (worker *externalWorker) VisualizationRegistrationConfig() visualization.RegistrationConfig {
	return visualization.RegistrationConfig{
		DisplayName: "Third-party Worker", Path: "workers/external",
		Mode: visualization.ModePull, PullInterval: time.Second,
	}
}

func main() {
	server := visualization.NewServer(visualization.VisualizationConfig{
		ServerAddress: "0.0.0.0:5005",
		TraceLimit:    50,
		PathPrefix:    "production",
	})
	if err := server.Start(); err != nil {
		panic(err)
	}
	defer func() { _ = visualization.ShutdownClientServer(context.Background()) }()

	doorData := &doorPayload{Location: "warehouse"}
	door := fsm.NewFSM("door", "closed", doorData)
	defer func() { _ = door.Close() }()
	door.SetTransitions(
		fsm.NewTransitionItem("open", "closed", "opened", fsm.Handler[*doorPayload]{Handler: func(machine *fsm.FSM[*doorPayload], _ ...any) error {
			machine.Payload.OpenCount++
			return nil
		}}),
		fsm.NewTransitionItem[*doorPayload]("close", "opened", "closed", fsm.Handler[*doorPayload]{}),
	)
	doorRegistration, err := visualization.RegisterClient(context.Background(), door)
	if err != nil {
		panic(err)
	}
	defer doorRegistration.Unregister()

	worker := &externalWorker{state: "idle"}
	workerRegistration, err := visualization.RegisterClient(context.Background(), worker)
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

func generateEvents(ctx context.Context, door *fsm.FSM[*doorPayload], worker *externalWorker) {
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
