package kvtx_block_okra

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// walTrackingStore is a block store that journals writes to the garbage
// collection WAL.
type walTrackingStore interface {
	// HasWALAppender reports whether writes append to the garbage collection WAL.
	HasWALAppender() bool
	// GetStore returns the underlying store without WAL tracking.
	GetStore() block.StoreOps
}

// stagedStore returns the staging store of the tree cursor's transaction. The
// tree transaction publishes staged blocks before it encodes the blocks that
// reference them.
func stagedStore(ctx context.Context, tree *block.Cursor) *block.BufferedStore {
	store, _ := tree.GetBlockStore()
	return tree.GetTransaction().StageWrites(ctx, valueMaterializationStore(store))
}

// stagedCursor returns a cursor in a private transaction that writes into the
// staging store of the tree cursor's transaction.
func stagedCursor(ctx context.Context, tree *block.Cursor) *block.Cursor {
	btx := tree.GetTransaction()
	staged := stagedStore(ctx, tree)
	stagedTx, cursor := block.NewTransaction(staged, btx.GetTransformer(), nil, btx.GetPutOpts())
	stagedTx.SetWriteBuffer(staged)
	return cursor
}

// writeStagedBlock writes a finished page or value block into the staging
// store of the tree cursor's transaction and returns its reference.
func writeStagedBlock(ctx context.Context, tree *block.Cursor, blk block.Block) (*block.BlockRef, error) {
	cursor := stagedCursor(ctx, tree)
	cursor.SetBlock(blk, true)
	ref, _, err := cursor.GetTransaction().WriteAtRoot(ctx, false, nil)
	return ref, err
}

// valueMaterializationStore returns the store to write staged blocks against:
// the untracked store while a GC WAL append is in progress, else the store.
func valueMaterializationStore(store block.StoreOps) block.StoreOps {
	if tracked, ok := store.(walTrackingStore); ok && tracked.HasWALAppender() {
		// The containing Okra page records each staged ref; avoid journaling
		// the eager write while a GC WAL append is in progress.
		return tracked.GetStore()
	}
	return store
}
