package fsm

import "github.com/swkings/fsm/ds"

type (
	Name            = ds.FSMName
	State           = ds.FSMState
	Event           = ds.FSMEvent
	EventStateKey   = ds.EventStateKey
	Metadata        = ds.Metadata
	TransitionTrace = ds.TransitionTrace
)

var (
	// EventEmpty indicates that no event is currently being processed.
	EventEmpty = ds.FSMEvent("")
	// StateEmpty indicates an absent or uninitialized state.
	StateEmpty = ds.FSMState("")
)
