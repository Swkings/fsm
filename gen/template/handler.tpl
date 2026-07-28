package handlers

import (
	"{{.FSMEnginePackage}}/fsm"
	"{{.PackageDir}}/{{.Package}}/types"
)

func _Before_s_{{.Transition.FromName}}_e_{{.Transition.EventName}}(f *fsm.FSM[types.Payload], _ ...any) error {
	// TODO: your code

	return nil
}

func _In_s_{{.Transition.FromName}}_e_{{.Transition.EventName}}(f *fsm.FSM[types.Payload], _ ...any) error {
	// TODO: your code

	return nil
}

func _After_s_{{.Transition.FromName}}_e_{{.Transition.EventName}}(f *fsm.FSM[types.Payload], _ ...any) error {
	// TODO: your code

	return nil
}
