package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os/signal"
	"syscall"
	"time"

	"github.com/swkings/fsm"
	"github.com/swkings/fsm/config"
	"github.com/swkings/fsm/examples/generated/player"
	"github.com/swkings/fsm/examples/generated/player/types"
	"github.com/swkings/fsm/store"
	"github.com/swkings/fsm/visualization"
	"github.com/zeromicro/go-zero/core/conf"
)

const (
	minEventInterval  = 3 * time.Second
	maxEventInterval  = 5 * time.Second
	generatedStoreKey = "generated/player"
)

type exampleConfig struct {
	FSM config.Config
}

func main() {
	configPath := flag.String("config", "examples/generated/config.yaml", "generated example config file")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *configPath); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, configPath string) error {
	appConfig, err := loadExampleConfig(configPath)
	if err != nil {
		return err
	}
	fsmStore, err := store.New(appConfig.FSM.Store)
	if err != nil {
		return fmt.Errorf("create fsm store: %w", err)
	}
	defer fsmStore.Close()
	clientServer := visualization.NewServer(appConfig.FSM.Visualization)
	if err = clientServer.RegisterReplayStore("primary", "Generated Player Store", fsmStore); err != nil {
		return fmt.Errorf("register replay store: %w", err)
	}
	if err := clientServer.Start(); err != nil {
		_ = visualization.ShutdownClientServer(context.Background())
		return fmt.Errorf("start visualization client server: %w", err)
	}
	defer visualization.ShutdownClientServer(context.Background())

	payload := &types.Payload{
		Name:         "示例角色",
		Health:       100,
		TargetHealth: 200,
		Stamina:      100,
	}
	machine := player.NewFSMPlayerAction(payload)
	defer machine.Close()
	if err = machine.BindStore(ctx, fsmStore, generatedStoreKey); err != nil {
		return fmt.Errorf("restore generated player: %w", err)
	}
	registration, err := visualization.RegisterClient(ctx, machine)
	if err != nil {
		return fmt.Errorf("register generated player visualization: %w", err)
	}
	defer registration.Unregister()

	fmt.Printf(
		"player=%s initial=%s health=%d stamina=%d\nvisualization=%s\n",
		payload.Name,
		machine.GetCurrentState(),
		payload.Health,
		payload.Stamina,
		clientServer.URL(),
	)
	return runRandomEvents(ctx, machine)
}

func loadExampleConfig(path string) (exampleConfig, error) {
	var appConfig exampleConfig
	if err := conf.Load(path, &appConfig); err != nil {
		return exampleConfig{}, fmt.Errorf("load generated example config %q: %w", path, err)
	}
	return appConfig, nil
}

func runRandomEvents(ctx context.Context, machine *fsm.FSM[*types.Payload]) error {
	for {
		events := machine.GetEventUnderState(machine.GetCurrentState())
		if len(events) == 0 {
			fmt.Printf("terminal state=%s; visualization remains available until shutdown\n", machine.GetCurrentState())
			<-ctx.Done()
			return nil
		}
		delay := randomEventInterval()
		event := events[rand.IntN(len(events))]
		params := randomEventParams(event)
		if len(params) > 0 {
			fmt.Printf("next random event=%s params=%+v in %s\n", event, params[0], delay)
		} else {
			fmt.Printf("next random event=%s in %s\n", event, delay)
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			if err := process(machine, event, params...); err != nil {
				return err
			}
		case <-ctx.Done():
			timer.Stop()
			return nil
		}
	}
}

func randomEventInterval() time.Duration {
	seconds := int(maxEventInterval/time.Second-minEventInterval/time.Second) + 1
	return minEventInterval + time.Duration(rand.IntN(seconds))*time.Second
}

func randomEventParams(event fsm.FSMEvent) []any {
	switch event {
	case types.EventEnum.HIT_LIGHT:
		return []any{types.AttackParams{Damage: 10 + rand.IntN(11)}}
	case types.EventEnum.HIT_HEAVY:
		return []any{types.AttackParams{Damage: 20 + rand.IntN(21)}}
	default:
		return nil
	}
}

func process(machine *fsm.FSM[*types.Payload], event fsm.FSMEvent, params ...any) error {
	trace, err := machine.ProcessEvent(event, params...)
	if err != nil {
		return fmt.Errorf("process %s from %s: %w", event, trace.FromState, err)
	}
	fmt.Printf(
		"%-13s -- %-10s --> %-13s health=%3d target=%3d damage=%2d stamina=%3d speed=%d vertical=%d\n",
		trace.FromState,
		trace.Event,
		trace.FinalState,
		machine.Payload.Health,
		machine.Payload.TargetHealth,
		machine.Payload.LastAttackDamage,
		machine.Payload.Stamina,
		machine.Payload.MoveSpeed,
		machine.Payload.VerticalSpeed,
	)
	return nil
}
