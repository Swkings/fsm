package handlers

import (
	"{{.FSMEnginePackage}}/fsm"
	"{{.PackageDir}}/{{.Package}}/types"
)

{{range $fsmName := .FSMNameList}}
// FSM: {{$fsmName}}
func _BeforeHandlerFor{{$fsmName}}_{{$.Transition.Event}}_{{$.Transition.From}}(f *fsm.FSM[types.Payload]) error {
	// TODO: your code

	return nil
}

func _HandlerFor{{$fsmName}}_{{$.Transition.Event}}_{{$.Transition.From}}(f *fsm.FSM[types.Payload]) error {
	// TODO: your code

	return nil
}

func _AfterHandlerFor{{$fsmName}}_{{$.Transition.Event}}_{{$.Transition.From}}(f *fsm.FSM[types.Payload]) error {
	// TODO: your code

	return nil
}

{{end}}