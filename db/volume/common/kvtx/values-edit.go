package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
)

// EditValues runs fn in one physical write transaction of the Volume's store
// and commits what it wrote. fn changes the block values through blocks, below
// the reference graph and any marking, and the object store storeID through
// tx, its view inside the same transaction, so both commit or discard
// together: the marker store of a block store lives in the key-value store
// that holds its blocks, as txMarks describes.
//
// Each attempt opens a fresh transaction, and fn runs again on a conflict or
// when retry allows, so fn must be safe to replay. A transaction fn left
// unchanged ends without a commit. The blocks fn removes reach the
// decoded-block cache once the transaction ends. The edit only removes or
// rewrites what the Volume holds, so it runs below the disk reserve too. It
// fails with block.ErrAtomicPublicationUnsupported when the store cannot
// commit atomically.
func (v *Volume) EditValues(
	ctx context.Context,
	storeID string,
	fn func(ctx context.Context, blocks block.StoreOps, tx kvtx.Tx) error,
	retry kvtx.RetryPredicate,
) error {
	// Require an atomic commit, join direct Volume operations and reject
	// edits after storage closes.
	if !v.SupportsAtomicPublication() {
		return block.ErrAtomicPublicationUnsupported
	}
	v.directMu.RLock()
	defer v.directMu.RUnlock()
	if v.directClosed {
		return block.ErrPublicationClosed
	}

	// Run fn on the physical transaction's blocks and the object store's
	// view, keeping the last attempt to settle.
	prefix := v.kvKey.GetObjectStorePrefixByID(storeID)
	var last *directTx
	open := func(ctx context.Context) (*directTx, error) {
		p, err := v.openPhysical(ctx)
		if err != nil {
			return nil, err
		}
		last = &directTx{v: v, p: p}
		return last, nil
	}
	err := kvtx.RunOptimisticTransaction(ctx, open, func(ctx context.Context, d *directTx) error {
		vtx, err := d.p.store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer vtx.Discard()
		return fn(ctx, d.p.blocks, kvtx_prefixer.NewTx(vtx, prefix))
	}, retry)

	// Wake Volume statistics watchers once the edit committed.
	if last != nil {
		last.settle()
	}
	return err
}
