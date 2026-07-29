package handlers

import (
	"github.com/swkings/fsm"
	"github.com/swkings/fsm/examples/generated/player/types"
)

func _Before_s_RUN_e_STOP(f *fsm.FSM[*types.Payload], _ ...any) error {
	return nil
}

func _In_s_RUN_e_STOP(f *fsm.FSM[*types.Payload], _ ...any) error {
	return handleTransition(f, types.EventEnum.STOP)
}

func _After_s_RUN_e_STOP(f *fsm.FSM[*types.Payload], _ ...any) error {
	return nil
}
