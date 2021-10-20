package world

import (
	"context"
)

// Engine implements a transactional world state container.
type Engine interface {
	// NewTransaction returns a new transaction against the store.
	// Indicate write if the transaction will not be read-only.
	// Always call Discard() after you are done with the transaction.
	// Check GetReadOnly, might not return a write tx if write=true.
	NewTransaction(write bool) (Tx, error)

	// WorldStorage provides access to the world storage via bucket cursors.
	WorldStorage

	// WaitSeqno waits for the seqno of the world state to be >= value.
	// Returns the seqno when the condition is reached.
	// If value == 0, this might return immediately unconditionally.
	WaitSeqno(ctx context.Context, value uint64) (uint64, error)
}
