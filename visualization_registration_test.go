package fsm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/swkings/fsm/visualization"
)

type visualizationMetadataPayload struct {
	Name     string `json:"name"`
	SubPath  string `json:"-"`
	Progress int    `json:"progress"`
}

func (payload visualizationMetadataPayload) JSON() ([]byte, error) {
	return json.Marshal(payload)
}

func (payload visualizationMetadataPayload) DisplayName() string { return payload.Name }
func (payload visualizationMetadataPayload) Category() string    { return payload.SubPath }

func TestRegisterClientUsesPayloadMetadataAndServerPathPrefix(t *testing.T) {
	_ = visualization.ShutdownClientServer(context.Background())
	t.Cleanup(func() { _ = visualization.ShutdownClientServer(context.Background()) })

	server := visualization.NewServer(visualization.VisualizationConfig{
		ServerAddress: "127.0.0.1:0",
		PathPrefix:    "production/players",
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	payload := &visualizationMetadataPayload{Name: "Player One", SubPath: "online"}
	machine := NewFSM("player", FSMState("idle"), payload)
	defer machine.Close()
	registration, err := visualization.RegisterClient(context.Background(), machine)
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()

	snapshot, exists := server.Snapshot(registration.ID())
	if !exists || snapshot.DisplayName != "Player One" ||
		snapshot.Path != "production/players/online" {
		t.Fatalf("payload registration metadata = %+v exists=%t", snapshot, exists)
	}
}

func TestRegisterClientAllowsEmptyPathPrefixAndPayloadCategory(t *testing.T) {
	_ = visualization.ShutdownClientServer(context.Background())
	t.Cleanup(func() { _ = visualization.ShutdownClientServer(context.Background()) })

	server := visualization.NewServer(visualization.VisualizationConfig{ServerAddress: "127.0.0.1:0"})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	payload := &visualizationMetadataPayload{Name: "Root Player"}
	machine := NewFSM("player", FSMState("idle"), payload)
	defer machine.Close()
	registration, err := visualization.RegisterClient(context.Background(), machine)
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()

	snapshot, exists := server.Snapshot(registration.ID())
	if !exists || snapshot.DisplayName != "Root Player" || snapshot.Path != "" {
		t.Fatalf("empty registration path = %+v exists=%t", snapshot, exists)
	}
}
