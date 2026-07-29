package main

import (
	"testing"
	"time"

	"github.com/swkings/fsm/examples/generated/player/types"
)

func TestGeneratedExampleLoadsVisualizationConfig(t *testing.T) {
	config, err := loadExampleConfig("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if config.Visualization.ServerAddress != "127.0.0.1:5005" ||
		config.Visualization.TraceLimit != 50 ||
		config.Visualization.PathPrefix != "examples/generated" {
		t.Fatalf("visualization config = %+v", config.Visualization)
	}
	if config.Store.JSONL.Directory != "data/fsm" || config.Store.JSONL.Filename != "generated-player.jsonl" {
		t.Fatalf("store config = %+v", config.Store)
	}
	if config.FSM.Name != "PlayerAction" || config.FSM.InitialState != "IDLE" || config.FSM.StoreKey != "generated/player" {
		t.Fatalf("fsm config = %+v", config.FSM)
	}
}

func TestGeneratedPayloadProvidesVisualizationMetadata(t *testing.T) {
	payload := types.Payload{Name: "Player One"}
	if payload.DisplayName() != "Player One" || payload.Category() != "player" {
		t.Fatalf(
			"payload visualization metadata = display:%q path:%q",
			payload.DisplayName(),
			payload.Category(),
		)
	}
}

func TestRandomEventIntervalIsBetweenThreeAndFiveSeconds(t *testing.T) {
	for range 1000 {
		interval := randomEventInterval()
		if interval < 3*time.Second || interval > 5*time.Second {
			t.Fatalf("random event interval = %s", interval)
		}
		if interval%time.Second != 0 {
			t.Fatalf("random event interval is not whole seconds: %s", interval)
		}
	}
}
