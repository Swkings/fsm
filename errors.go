package fsm

import (
	"errors"
	"fmt"
)

var (
	// ErrorEventStateKeyInvalid reports that no rule accepts an event in the current state.
	ErrorEventStateKeyInvalid = func(key EventStateKey) error {
		return fmt.Errorf("event state key %v not exist", key.String())
	}
	// ErrorEventStateKeyRepeated reports a duplicate transition registration.
	ErrorEventStateKeyRepeated = func(key EventStateKey) error {
		return fmt.Errorf("event state key %v is repeated", key.String())
	}
	ErrorSubFSMInvalid      = errors.New("sub fsm invalid")
	ErrorSubFSMSameAsFather = errors.New("sub fsm can not same as father fsm")
	ErrorEventProcessing    = errors.New("fsm event is processing")
	ErrorStateChanged       = errors.New("fsm state changed during event processing")
)
