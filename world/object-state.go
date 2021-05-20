package world

import "github.com/aperturerobotics/hydra/bucket"

// ObjectState contains the object state interface.
// Represents a handle a object in the store.
type ObjectState interface {
	// GetRootRef returns the root reference.
	GetRootRef() (*bucket.ObjectRef, error)
	// SetRootRef changes the root reference of the object.
	SetRootRef(nref *bucket.ObjectRef) error
	// ApplyOperation applies an object-specific operation.
	// Returns any errors processing the operation.
	ApplyOperation(op ObjectOp) error
}
