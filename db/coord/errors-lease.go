package coord

import "errors"

var (
	// ErrLeaseReleased indicates a write lease has already been released.
	ErrLeaseReleased = errors.New("coord: lease released")
	// ErrStaleGeneration indicates a writer published from an old generation.
	ErrStaleGeneration = errors.New("coord: stale generation")
	// ErrScopeEmpty indicates a scope with no ObjectStoreID and no Key.
	ErrScopeEmpty = errors.New("coord: scope has no object store id and no key")
)
