package handlers

import (
	"{{.FSMEnginePackage}}/fsm"
	"{{.PackageDir}}/{{.Package}}/types"
)

func _BeforeHandler_{{.Transition.Event}}_{{.Transition.From}}(f *fsm.FSM[types.Payload]) error {
	// TODO: your code

	return nil
}

func _Handler_{{.Transition.Event}}_{{.Transition.From}}(f *fsm.FSM[types.Payload]) error {
	// TODO: your code

	return nil
}

func _AfterHandler_{{.Transition.Event}}_{{.Transition.From}}(f *fsm.FSM[types.Payload]) error {
	// TODO: your code

	return nil
}
