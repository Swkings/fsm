package handlers

import (
	"github.com/swkings/fsm"
	"github.com/swkings/fsm/examples/generated/player/types"
)

func _Before_s_ATTACK_LIGHT_e_DAMAGE(f *fsm.FSM[types.Payload], _ ...any) error {
	return nil
}

func _In_s_ATTACK_LIGHT_e_DAMAGE(f *fsm.FSM[types.Payload], _ ...any) error {
	return handleTransition(f, types.EventEnum.DAMAGE)
}

func _After_s_ATTACK_LIGHT_e_DAMAGE(f *fsm.FSM[types.Payload], _ ...any) error {
	return nil
}
