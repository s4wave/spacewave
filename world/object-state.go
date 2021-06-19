package world

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
)

// ObjectState contains the object state interface.
// Represents a handle a object in the store.
type ObjectState interface {
	// GetKey returns the key this state object is for.
	GetKey() string
	// GetRootRef returns the root reference.
	// Returns the revision number.
	GetRootRef() (*bucket.ObjectRef, uint64, error)

	// AccessWorldState builds a bucket lookup cursor with an optional ref.
	// If the ref is empty, will default to the object RootRef.
	// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
	// The lookup cursor will be released after cb returns.
	AccessWorldState(
		ctx context.Context,
		write bool,
		ref *bucket.ObjectRef,
		cb func(*bucket_lookup.Cursor) error,
	) error

	// SetRootRef changes the root reference of the object.
	// Increments the revision of the object if changed.
	// Returns revision just after the change was applied.
	SetRootRef(nref *bucket.ObjectRef) (uint64, error)
	// ApplyObjectOp applies a batch operation at the object level.
	// The handling of the operation is operation-type specific.
	// Returns the revision following the operation execution.
	// If nil is returned for the error, implies success.
	ApplyObjectOp(operationTypeID string, op Operation) (uint64, error)

	// IncrementRev increments the revision of the object.
	// Returns revision just after the change was applied.
	IncrementRev() (uint64, error)
	// WaitRev waits until the object rev is >= the specified.
	// Returns ErrObjectNotFound if the object is deleted.
	// If ignoreNotFound is set, waits for the object to exist.
	// Returns the new rev.
	WaitRev(ctx context.Context, rev uint64, ignoreNotFound bool) (uint64, error)
}
