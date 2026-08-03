package server_test

import (
	"context"
	"errors"
	"testing"

	"github.com/swkings/fsm"
	"github.com/swkings/fsm/config"
	fsmserver "github.com/swkings/fsm/server"
)

func TestServerIsProcessWideSingletonUntilShutdown(t *testing.T) {
	shutdownGlobalServer(t)
	t.Cleanup(func() { shutdownGlobalServer(t) })

	first, err := fsmserver.NewServer(config.Config{})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	second, err := fsmserver.NewServer(config.Config{
		Visualization: config.Visualization{ServerAddress: "127.0.0.1:0"},
	})
	if err != nil {
		t.Fatalf("second NewServer() error = %v", err)
	}
	if second != first {
		t.Fatal("NewServer() returned different process-wide instances")
	}
	current, exists := fsmserver.GlobalServer()
	if !exists || current != first {
		t.Fatalf("GlobalServer() = (%p, %t), want (%p, true)", current, exists, first)
	}
	if err = first.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if current, exists = fsmserver.GlobalServer(); exists || current != nil {
		t.Fatalf("GlobalServer() after shutdown = (%p, %t), want (nil, false)", current, exists)
	}

	recreated, err := fsmserver.NewServer(config.Config{})
	if err != nil {
		t.Fatalf("recreate NewServer() error = %v", err)
	}
	if recreated == first {
		t.Fatal("NewServer() reused a stopped Server")
	}
	if err = recreated.Shutdown(context.Background()); err != nil {
		t.Fatalf("recreated Shutdown() error = %v", err)
	}
}

func TestFSMRegisterUsesGlobalServerWhenOmitted(t *testing.T) {
	shutdownGlobalServer(t)
	t.Cleanup(func() { shutdownGlobalServer(t) })
	machine := fsm.NewFSM("global-register", "idle", &testPayload{})
	t.Cleanup(func() { _ = machine.Close() })

	if err := machine.Register(); !errors.Is(err, fsmserver.ErrServerNotConfigured) {
		t.Fatalf("Register() without Server error = %v, want %v", err, fsmserver.ErrServerNotConfigured)
	}
	server := fsmserver.MustNewServer(config.Config{})
	if err := machine.Register(); err != nil {
		t.Fatalf("Register() with global Server error = %v", err)
	}
	if !server.Unregister(machine.InstanceID) {
		t.Fatal("global Server did not contain registered FSM")
	}
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func shutdownGlobalServer(t *testing.T) {
	t.Helper()
	if server, exists := fsmserver.GlobalServer(); exists {
		if err := server.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown existing global Server: %v", err)
		}
	}
}
