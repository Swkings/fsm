package handlers

import (
	"github.com/swkings/fsm"
	"github.com/swkings/fsm/examples/generated/player/types"
)

func _Before_s_WALK_e_HIT_HEAVY(f *fsm.FSM[*types.Payload], _ ...any) error {
	return nil
}

func _In_s_WALK_e_HIT_HEAVY(f *fsm.FSM[*types.Payload], params ...any) error {
	return handleAttackTransition(f, types.EventEnum.HIT_HEAVY, params...)
}

func _After_s_WALK_e_HIT_HEAVY(f *fsm.FSM[*types.Payload], _ ...any) error {
	return nil
}
