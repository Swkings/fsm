// Package store provides FSM persistence independently from visualization.
package store

import (
	"context"
	"errors"

	storeds "github.com/swkings/fsm/store/ds"
)

// ErrNotFound indicates that no record matched the requested key or ID.
var ErrNotFound = errors.New("fsm store record not found")

type (
	// Metadata is the convenient store-package alias for ds.Metadata.
	Metadata = storeds.Metadata
	// Transition is the convenient store-package alias for ds.Transition.
	Transition = storeds.Transition
	// Trace is the convenient store-package alias for ds.Trace.
	Trace = storeds.Trace
	// Record is the convenient store-package alias for ds.Record.
	Record = storeds.Record
	// Query is the convenient store-package alias for ds.Query.
	Query = storeds.Query
	// Descriptor is the convenient store-package alias for ds.Descriptor.
	Descriptor = storeds.Descriptor
	// RemoteDirectoryEntry is the convenient store-package alias for ds.RemoteDirectoryEntry.
	RemoteDirectoryEntry = storeds.RemoteDirectoryEntry
	// RemoteDirectoryView is the convenient store-package alias for ds.RemoteDirectoryView.
	RemoteDirectoryView = storeds.RemoteDirectoryView
)

// StoreI is the common contract implemented by all FSM persistence backends.
type StoreI interface {
	Save(context.Context, Record) error
	List(context.Context, Query) ([]Record, error)
	Latest(context.Context, string) (Record, error)
	Get(context.Context, string) (Record, error)
	Delete(context.Context, string) error
	Descriptor() Descriptor
	Close() error
}
