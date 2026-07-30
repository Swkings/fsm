package fsm

import commonds "github.com/swkings/fsm/ds"

// NewTransitionTrace creates a transition trace through the root convenience API.
func NewTransitionTrace(prevTtID, ttID string, fromState FSMState, event FSMEvent) *TransitionTrace {
	return commonds.NewTransitionTrace(prevTtID, ttID, fromState, event)
}
