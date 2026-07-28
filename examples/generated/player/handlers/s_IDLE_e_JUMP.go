package handlers

import (
	"github.com/swkings/fsm"
	"github.com/swkings/fsm/examples/generated/player/types"
)

func _Before_s_IDLE_e_JUMP(f *fsm.FSM[types.Payload], _ ...any) error {
	return nil
}

func _In_s_IDLE_e_JUMP(f *fsm.FSM[types.Payload], _ ...any) error {
	return handleTransition(f, types.EventEnum.JUMP)
}

func _After_s_IDLE_e_JUMP(f *fsm.FSM[types.Payload], _ ...any) error {
	return nil
}
