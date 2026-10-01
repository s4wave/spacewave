package world

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
)

// Engine implements a transactional world state container.
type Engine interface {
	// OperationAuthor returns this engine's signing device and its accepted person.
	// Unsigned engines return an empty device; callers requiring authorship reject it.
	OperationAuthor(ctx context.Context) (peer.ID, string, error)
	// NewTransaction returns a new transaction against the store.
	// A read transaction retains one immutable revision until Discard. Use
	// NewEngineWorldState for live reads and object revision watches.
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

	// WaitObjectRev waits until the object at key reaches rev and returns its
	// revision. Returns ErrObjectNotFound if the object does not exist, unless
	// ignoreNotFound is set, in which case it waits for the object to appear.
	// Use WaitObjectRevBySeqno when the engine cannot watch individual keys.
	WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error)
}

// RootRetainingEngine is an Engine that retains named World roots in its
// shared state, so storage reclaim keeps their blocks for recovery.
type RootRetainingEngine interface {
	// SetRetainedRoot retains the World root ref under name, replacing the
	// root the name held. ref must be the accepted head, with its blocks in
	// the World's storage. An empty ref releases the name.
	SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error
}

// EngineResolver is a function which resolves an engine for a ref count.
type EngineResolver = refcount.RefCountResolver[*Engine]
