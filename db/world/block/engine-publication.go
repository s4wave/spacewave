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
	if err := ref.Validate(); err != nil {
		return err
	}
	cursor, err := e.baseRoot.FollowRef(ctx, ref)
	if err != nil {
		return err
	}
	defer cursor.Release()
	_, bcs := cursor.BuildTransactionWithStore(nil, store)
	ws, err := NewWorldState(ctx, e.le, false, nil, bcs, store, cursor.GetTransformer(), nil, nil, e.lookupOp, e.verbose)
	if err != nil {
		return err
	}
	defer ws.Discard()
	_, err = ws.objTree.Size(ctx)
	return err
}
