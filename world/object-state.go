package world

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
)

// ObjectState contains the object state interface.
// Represents a handle a object in the store.
type ObjectState interface {
	// GetRootRef returns the root reference.
	// Returns the revision number.
	GetRootRef() (*bucket.ObjectRef, uint64, error)
	// SetRootRef changes the root reference of the object.
	// Increments the revision of the object if changed.
	// Returns revision just after the change was applied.
	SetRootRef(nref *bucket.ObjectRef) (uint64, error)
	// ApplyOperation applies an object-specific operation.
	// Returns any errors processing the operation.
	// Returns revision just after the change was applied.
	ApplyOperation(op ObjectOp) (uint64, error)
	// IncrementRev increments the revision of the object.
	// Returns revision just after the change was applied.
	IncrementRev() (uint64, error)
	// WaitRev waits until the object rev is >= the specified.
	// Returns ErrObjectNotFound if the object is deleted.
	// If ignoreNotFound is set, waits for the object to exist.
	// Returns the new rev.
	WaitRev(ctx context.Context, rev uint64, ignoreNotFound bool) (uint64, error)
}
