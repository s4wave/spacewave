package world

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
)

// Engine implements a transactional world state container.
type Engine interface {
	// NewTransaction returns a new transaction against the store.
	// Indicate write if the transaction will not be read-only.
	// Always call Discard() after you are done with the transaction.
	// Check GetReadOnly, might not return a write tx if write=true.
	NewTransaction(ctx context.Context, write bool) (Tx, error)

	// WorldStorage provides access to the world storage via bucket cursors.
	WorldStorage

	// WorldWaitSeqno allows waiting for the world seqno to change.
	WorldWaitSeqno
}

// EngineResolver is a function which resolves an engine for a ref count.
type EngineResolver = refcount.RefCountResolver[*Engine]
