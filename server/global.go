package server

import (
	"sync"

	"github.com/swkings/fsm/config"
)

var globalServer struct {
	sync.RWMutex
	instance *Server
}

// NewServer creates the process-wide FSM Server, or returns the existing one.
// The configuration from the first successful call remains effective until
// that Server is shut down.
func NewServer(cfg config.Config) (*Server, error) {
	globalServer.Lock()
	defer globalServer.Unlock()
	if globalServer.instance != nil {
		return globalServer.instance, nil
	}
	instance, err := newServer(cfg)
	if err != nil {
		return nil, err
	}
	globalServer.instance = instance

	return instance, nil
}

// GlobalServer returns the process-wide FSM Server after it has been created.
func GlobalServer() (*Server, bool) {
	globalServer.RLock()
	defer globalServer.RUnlock()

	return globalServer.instance, globalServer.instance != nil
}

func clearGlobalServer(expected *Server) {
	globalServer.Lock()
	if globalServer.instance == expected {
		globalServer.instance = nil
	}
	globalServer.Unlock()
}
