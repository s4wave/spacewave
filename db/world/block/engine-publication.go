package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// validatePreparedRoot reads through the publication's transaction-scoped
// overlay, never through an independently opened physical read transaction.
// The engine's commit lifetime retains baseRoot until this callback completes.
// This must not acquire the Engine guard or write to the supplied store.
func (e *Engine) validatePreparedRoot(ctx context.Context, ref *bucket.ObjectRef, store block.StoreOps) error {
	// Validate the reference the caller resolved before following it.
	if err := ref.Validate(); err != nil {
		return err
	}

	// Follow the reference through the base root into a transaction cursor.
	cursor, err := e.baseRoot.FollowRef(ctx, ref)
	if err != nil {
		return err
	}
	defer cursor.Release()

	// Build the publication's transaction-scoped block store on the cursor.
	_, bcs := cursor.BuildTransactionWithStore(nil, store)

	// Open a read-only WorldState over that store.
	ws, err := NewWorldState(ctx, e.le, false, nil, bcs, store, nil, e.lookupOp, e.verbose)
	if err != nil {
		return err
	}
	defer ws.Discard()

	// Read the object tree size so validation walks the real blocks.
	_, err = ws.objTree.Size(ctx)
	return err
}
