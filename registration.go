package fsm

import (
	"errors"

	fsmserver "github.com/swkings/fsm/server"
)

// RegisterTo registers the FSM with a configuration-driven server.
func (fsm *FSM[T]) Register(server *fsmserver.Server) error {
	if server == nil {
		return errors.New("fsm server is nil")
	}
	return server.Register(fsm)
}
