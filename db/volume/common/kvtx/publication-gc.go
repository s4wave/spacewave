package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
)

// withDirectAtomic joins direct preparation/sweep calls on Close and uses the
// same raw physical transaction domain as the queued writer. It deliberately
// does not acquire a coordinator lease or an Engine publication guard.
func (v *Volume) withDirectAtomic(ctx context.Context, fn func(block.StoreOps, *block_gc.RefGraph) (bool, error)) error {
	v.directMu.RLock()
	defer v.directMu.RUnlock()
	if v.directClosed {
		return block.ErrPublicationClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := v.kvtxStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	store := kvtx.NewTxStore(tx)
	blocks := block_store_kvtx.NewKVTxBlock(v.kvKey, store, v.GetHashType(), v.atomicHashGet)
	rg, err := block_gc.NewRefGraph(ctx, store, volumeRefGraphPrefix())
	if err != nil {
		return err
	}
	defer rg.Close()
	changed, err := fn(blocks, rg)
	if err != nil || !changed {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	v.broadcastStorageStatsChanged()
	return nil
}

// PrepareOwnedBlock durably prepares a potentially oversized body together with
// its bucket ownership. Unlike queue admission this call retains no caller data
// after return; it also checks existence inside the physical write transaction.
func (v *Volume) PrepareOwnedBlock(ctx context.Context, bucketID string, data []byte, opts *block.PutOpts) (ref *block.BlockRef, existed bool, err error) {
	if !v.SupportsAtomicPublication() {
		return nil, false, block.ErrAtomicPublicationUnsupported
	}
	err = v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		putOpts, _ := block.PutOptsWithoutSync(opts)
		if bucketID == "" {
			ref, existed, err = blocks.PutBlock(ctx, data, putOpts)
			return err == nil, err
		}
		owner := block_gc.BucketIRI(bucketID)
		if err := rg.AddRef(ctx, block_gc.NodeGCRoot, owner); err != nil {
			return false, err
		}
		gc := block_gc.NewGCStoreOpsWithParentAndTraceTask(blocks, rg, owner, block_gc.BucketFlushTask())
		ref, existed, err = gc.PutBlock(ctx, data, putOpts)
		if err == nil {
			err = gc.FlushPending(ctx)
		}
		return err == nil, err
	})
	return
}

// PrepareOwnedBlockBatch is used by bounded staging's ordinary capacity drains.
// It cannot expose a gap between persisting bytes and rescuing their ownership.
func (v *Volume) PrepareOwnedBlockBatch(ctx context.Context, bucketID string, entries []*block.PutBatchEntry) error {
	if !v.SupportsAtomicPublication() {
		return block.ErrAtomicPublicationUnsupported
	}
	return v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		if len(entries) == 0 {
			return false, nil
		}
		if bucketID == "" {
			err := blocks.PutBlockBatch(ctx, entries)
			return err == nil, err
		}
		owner := block_gc.BucketIRI(bucketID)
		if err := rg.AddRef(ctx, block_gc.NodeGCRoot, owner); err != nil {
			return false, err
		}
		gc := block_gc.NewGCStoreOpsWithParentAndTraceTask(blocks, rg, owner, block_gc.BucketFlushTask())
		err := gc.PutBlockBatch(ctx, entries)
		if err == nil {
			err = gc.FlushPending(ctx)
		}
		return err == nil, err
	})
}

// SweepUnreferenced treats candidate snapshots as hints, not deletion authority.
// Current owners, outgoing edges, orphan markers, and physical deletion for the
// whole batch share one raw transaction with the same serialization as grouped
// head publication.
func (v *Volume) SweepUnreferenced(ctx context.Context, graph block_gc.RefGraphOps, nodes []string) ([]string, error) {
	actual, ok := v.refGraph.(*transactionRefGraph)
	given, sameType := graph.(*transactionRefGraph)
	if !ok || !sameType || given != actual || !v.SupportsAtomicPublication() {
		return nil, block_gc.ErrAtomicSweepUnsupported
	}
	var swept []string
	err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		for _, node := range nodes {
			removed, err := sweepOrphan(ctx, blocks, rg, node)
			if err != nil {
				return false, err
			}
			if removed {
				swept = append(swept, node)
			}
		}
		return len(swept) != 0, nil
	})
	// A failed physical transaction must not report a successful sweep.
	if err != nil {
		return nil, err
	}
	return swept, nil
}

// sweepOrphan removes node inside the sweep transaction when its only current
// owner is the unreferenced marker.
func sweepOrphan(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, node string) (bool, error) {
	if block_gc.IsPermanentRoot(node) {
		return false, nil
	}
	incoming, err := rg.GetIncomingRefs(ctx, node)
	if err != nil {
		return false, err
	}
	marked := false
	for _, owner := range incoming {
		if owner != block_gc.NodeUnreferenced {
			return false, nil // rescued after the collector took its snapshot
		}
		marked = true
	}
	if !marked {
		return false, nil // already swept, or not an orphan candidate
	}
	if _, err := rg.RemoveNodeRefs(ctx, node, true); err != nil {
		return false, err
	}
	if err := rg.RemoveRef(ctx, block_gc.NodeUnreferenced, node); err != nil {
		return false, err
	}
	if ref, ok := block_gc.ParseBlockIRI(node); ok {
		if err := blocks.RmBlock(ctx, ref); err != nil {
			return false, err
		}
	}
	return true, nil
}

var (
	_ block_gc.AtomicSweepStore = (*Volume)(nil)
	_ block.AtomicBlockPreparer = (*Volume)(nil)
)
