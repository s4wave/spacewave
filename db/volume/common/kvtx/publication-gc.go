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
//
// It commits with write ordering when the store supports it. Every direct
// stage is safe to lose in a crash as long as no earlier commit is lost:
// prepared blocks stay owned by their bucket or stage, and root proofs, pins and sweeps
// are recomputed or redone. The head publication that follows commits fully
// and makes these writes durable, as does Sync.
func (v *Volume) withDirectAtomic(ctx context.Context, fn func(block.StoreOps, *block_gc.RefGraph) (bool, error)) error {
	// Join direct Volume operations and reject writes after storage closes.
	v.directMu.RLock()
	defer v.directMu.RUnlock()
	if v.directClosed {
		return block.ErrPublicationClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Open one physical transaction for block bytes and reference changes.
	tx, err := v.kvtxStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Open transaction-local block and reference graph adapters.
	store := kvtx.NewTxStore(tx)
	blocks := block_store_kvtx.NewKVTxBlock(v.kvKey, store, v.GetHashType(), v.atomicHashGet)
	rg, err := block_gc.NewRefGraph(ctx, store, volumeRefGraphPrefix())
	if err != nil {
		return err
	}
	defer rg.Close()

	// Apply the direct operation and skip unchanged transactions.
	changed, err := fn(blocks, rg)
	if err != nil || !changed {
		return err
	}

	// Commit the direct changes using the Volume write-ordering contract.
	if v.ordered != nil {
		err = kvtx.CommitOrdered(ctx, tx)
	} else {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return err
	}

	// Wake Volume statistics watchers after the committed changes.
	v.broadcastStorageStatsChanged()
	return nil
}

// PrepareOwnedBlock durably prepares a potentially oversized body together with
// its bucket ownership. Unlike queue admission this call retains no caller data
// after return; it also checks existence inside the physical write transaction.
// An empty bucketID writes the block without ownership.
func (v *Volume) PrepareOwnedBlock(ctx context.Context, bucketID string, data []byte, opts *block.PutOpts) (ref *block.BlockRef, existed bool, err error) {
	putOpts, _ := block.PutOptsWithoutSync(opts)
	err = v.prepareOwned(ctx, bucketOwner(bucketID), claimBucket, func(store block.StoreOps) error {
		ref, existed, err = store.PutBlock(ctx, data, putOpts)
		return err
	})
	return ref, existed, err
}

// PrepareOwnedBlockBatch is used by bounded staging's ordinary capacity drains.
// It cannot expose a gap between persisting bytes and rescuing their ownership.
func (v *Volume) PrepareOwnedBlockBatch(ctx context.Context, bucketID string, entries []*block.PutBatchEntry) error {
	if len(entries) == 0 && v.SupportsAtomicPublication() {
		return nil
	}
	return v.prepareOwned(ctx, bucketOwner(bucketID), claimBucket, func(store block.StoreOps) error {
		return store.PutBlockBatch(ctx, entries)
	})
}

// bucketOwner returns the graph node of a bucket, or empty for no bucket.
func bucketOwner(bucketID string) string {
	if bucketID == "" {
		return ""
	}
	return block_gc.BucketIRI(bucketID)
}

// claimBucket retains the bucket as a permanent garbage collection root.
func claimBucket(ctx context.Context, rg *block_gc.RefGraph, bucket string) error {
	return rg.AddRef(ctx, block_gc.NodeGCRoot, bucket)
}

// prepareOwned writes blocks and their ownership by owner in one physical
// transaction. claim checks or roots the owner first. An empty owner writes the
// blocks without ownership.
func (v *Volume) prepareOwned(
	ctx context.Context,
	owner string,
	claim func(context.Context, *block_gc.RefGraph, string) error,
	put func(block.StoreOps) error,
) error {
	if !v.SupportsAtomicPublication() {
		return block.ErrAtomicPublicationUnsupported
	}
	return v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		// Write unowned blocks directly in the shared transaction.
		if owner == "" {
			err := put(blocks)
			return err == nil, err
		}

		// Claim the owner, then write the blocks and flush their ownership
		// together.
		if err := claim(ctx, rg, owner); err != nil {
			return false, err
		}
		gc := block_gc.NewGCStoreOpsWithParentAndTraceTask(blocks, rg, owner, block_gc.BucketFlushTask())
		err := put(gc)
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
//
// The batch releases its edges in one ownership transition. The orphan marker
// owns every candidate, so releasing nodes one at a time rewrites its posting
// list once per node, which is quadratic in the number of orphans.
func (v *Volume) SweepUnreferenced(ctx context.Context, graph block_gc.RefGraphOps, nodes []string) ([]string, error) {
	// Require the Volume transaction domain for atomic sweeping.
	actual, ok := v.refGraph.(*transactionRefGraph)
	given, sameType := graph.(*transactionRefGraph)
	if !ok || !sameType || given != actual || !v.SupportsAtomicPublication() {
		return nil, block_gc.ErrAtomicSweepUnsupported
	}

	// Sweep candidates and their graph edges in one physical transaction.
	var swept []string
	err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		// Collect the candidates the marker still owns alone, with their edges.
		swept = swept[:0]
		var removes []block_gc.RefEdge
		seen := make(map[string]struct{}, len(nodes))
		for _, node := range nodes {
			if _, dup := seen[node]; dup {
				continue
			}
			seen[node] = struct{}{}
			orphan, err := isMarkedOrphan(ctx, rg, node)
			if err != nil {
				return false, err
			}
			if !orphan {
				continue
			}
			targets, err := rg.GetOutgoingRefs(ctx, node)
			if err != nil {
				return false, err
			}
			for _, target := range targets {
				removes = append(removes, block_gc.RefEdge{Subject: node, Object: target})
			}
			removes = append(removes, block_gc.RefEdge{Subject: block_gc.NodeUnreferenced, Object: node})
			swept = append(swept, node)
		}
		if len(swept) == 0 {
			return false, nil
		}

		// Release the edges together. Removing a node's own marker keeps it
		// unmarked, and each child left without an owner gains a marker.
		if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
			return false, err
		}

		// Delete the swept blocks.
		for _, node := range swept {
			if ref, ok := block_gc.ParseBlockIRI(node); ok {
				if err := blocks.RmBlock(ctx, ref); err != nil {
					return false, err
				}
			}
		}
		return true, nil
	})

	// A failed physical transaction must not report a successful sweep.
	if err != nil {
		return nil, err
	}
	return swept, nil
}

// isMarkedOrphan reports whether the unreferenced marker is the only current
// owner of node.
func isMarkedOrphan(ctx context.Context, rg *block_gc.RefGraph, node string) (bool, error) {
	// A permanent root is never swept.
	if block_gc.IsPermanentRoot(node) {
		return false, nil
	}

	// Any other owner rescued the node after the collector took its snapshot.
	// Without the marker it was already swept or never a candidate.
	incoming, err := rg.GetIncomingRefs(ctx, node)
	if err != nil {
		return false, err
	}
	marked := false
	for _, owner := range incoming {
		if owner != block_gc.NodeUnreferenced {
			return false, nil
		}
		marked = true
	}
	return marked, nil
}

var (
	_ block_gc.AtomicSweepStore = (*Volume)(nil)
	_ block.AtomicBlockPreparer = (*Volume)(nil)
)
