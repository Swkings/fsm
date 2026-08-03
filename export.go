package fsm

import "github.com/swkings/fsm/ds"

type (
	Name            = ds.FSMName
	State           = ds.FSMState
	Event           = ds.FSMEvent
	EventStateKey   = ds.EventStateKey
	Metadata        = ds.Metadata
	OperationBy     = ds.OperationBy
	TransitionTrace = ds.TransitionTrace
)

const (
	OperationByAuto   = ds.OperationByAuto
	OperationByManual = ds.OperationByManual
)

var (
	// EventEmpty indicates that no event is currently being processed.
	EventEmpty = ds.FSMEvent("")
	// StateEmpty indicates an absent or uninitialized state.
	StateEmpty = ds.FSMState("")
)
