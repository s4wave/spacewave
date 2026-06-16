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

	// Sync fences durable storage and advances the durable world head.
	// Runs the block barrier so every block written so far is durable, then
	// commits the current in-memory root to the durable head ordered after the
	// barrier. Returns true if a durability fence was applied, false if the
	// store is always-durable and no fence was required.
	Sync(ctx context.Context) (bool, error)

	// WorldStorage provides access to the world storage via bucket cursors.
	WorldStorage

	// WorldWaitSeqno allows waiting for the world seqno to change.
	WorldWaitSeqno
}

// EngineResolver is a function which resolves an engine for a ref count.
type EngineResolver = refcount.RefCountResolver[*Engine]
