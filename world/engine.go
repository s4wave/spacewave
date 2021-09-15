package world

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
)

// Engine implements a transactional world state container.
type Engine interface {
	// NewTransaction returns a new transaction against the store.
	// Indicate write if the transaction will not be read-only.
	// Always call Discard() after you are done with the transaction.
	// Check GetReadOnly, might not return a write tx if write=true.
	NewTransaction(write bool) (Tx, error)
	// AccessWorldState builds a bucket lookup cursor with an optional ref.
	// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
	// The lookup cursor will be released after cb returns.
	AccessWorldState(
		ctx context.Context,
		ref *bucket.ObjectRef,
		cb func(*bucket_lookup.Cursor) error,
	) error
	// WaitSeqno waits for the seqno of the world state to be >= value.
	// Returns the seqno when the condition is reached.
	// If value == 0, this might return immediately unconditionally.
	WaitSeqno(ctx context.Context, value uint64) (uint64, error)
}
