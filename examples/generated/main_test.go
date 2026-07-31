package main

import (
	"testing"

	"github.com/swkings/fsm"
	"github.com/swkings/fsm/examples/generated/player"
	"github.com/swkings/fsm/examples/generated/player/types"
)

func TestGeneratedPlayerTransitionGraphAndLogic(t *testing.T) {
	payload := &types.Payload{Name: "test-player", Health: 100, TargetHealth: 100, Stamina: 100}
	machine := player.NewFSMPlayerAction(payload)
	defer func() { _ = machine.Close() }()

	transitionCounts := map[fsm.State]int{
		types.StateEnum.IDLE:         7,
		types.StateEnum.WALK:         7,
		types.StateEnum.RUN:          5,
		types.StateEnum.JUMP_START:   1,
		types.StateEnum.IN_AIR:       3,
		types.StateEnum.ATTACK_LIGHT: 3,
		types.StateEnum.ATTACK_HEAVY: 3,
		types.StateEnum.HURT:         2,
		types.StateEnum.DIE:          1,
	}
	for state, want := range transitionCounts {
		if got := len(machine.GetEventUnderState(state)); got != want {
			t.Errorf("transitions under %s = %d, want %d", state, got, want)
		}
	}

	sequence := []struct {
		event  fsm.Event
		params []any
	}{
		{event: types.EventEnum.MOVE},
		{event: types.EventEnum.RUN},
		{event: types.EventEnum.JUMP},
		{event: types.EventEnum.ANIM_END},
		{event: types.EventEnum.LAND},
		{event: types.EventEnum.HIT_HEAVY, params: []any{types.AttackParams{Damage: 25}}},
		{event: types.EventEnum.DAMAGE},
		{event: types.EventEnum.DEAD},
		{event: types.EventEnum.REVIVE},
	}
	for _, item := range sequence {
		if _, err := machine.ProcessEvent(item.event, item.params...); err != nil {
			t.Fatalf("process %s: %v", item.event, err)
		}
	}

	if state := machine.GetCurrentState(); state != types.StateEnum.IDLE || state.String() != "IDLE" {
		t.Fatalf("final state = %q", state)
	}
	if payload.Health != 100 || payload.TargetHealth != 75 || payload.Stamina != 100 ||
		payload.LastAttackDamage != 25 || payload.ActionCount != len(sequence) {
		t.Fatalf("final payload = %+v", payload)
	}
}

func TestGeneratedPlayerAttackRequiresDamageParams(t *testing.T) {
	payload := &types.Payload{Name: "test-player", Health: 100, TargetHealth: 100, Stamina: 100}
	machine := player.NewFSMPlayerAction(payload)
	defer func() { _ = machine.Close() }()

	if _, err := machine.ProcessEvent(types.EventEnum.HIT_LIGHT); err == nil {
		t.Fatal("HIT_LIGHT without AttackParams error = nil")
	}
	if state := machine.GetCurrentState(); state != types.StateEnum.IDLE {
		t.Fatalf("state after rejected attack = %s", state)
	}
	if payload.TargetHealth != 100 || payload.Stamina != 100 || payload.ActionCount != 0 {
		t.Fatalf("rejected attack changed payload = %+v", payload)
	}
}
