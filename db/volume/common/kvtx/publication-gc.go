package kvtx

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
)

// withDirectAtomic joins direct preparation and sweep calls on Close and uses
// the same raw physical transaction domain as the queued writer. It
// deliberately does not acquire a coordinator lease or an Engine publication
// guard.
//
// Every commit costs a write barrier, so a transaction fn leaves unchanged ends
// without one. A changing transaction also carries the Volume's deferred edits,
// the edges of released reader pins and pending root proofs, which therefore
// cost no barrier of their own. A nil fn applies only the deferred edits.
//
// It commits with write ordering when the store supports it. Every direct
// stage is safe to lose in a crash as long as no earlier commit is lost:
// prepared blocks stay owned by their bucket or stage, and root proofs, pins and
// sweeps are recomputed or redone. The head publication that follows commits
// fully and makes these writes durable, as does Sync.
func (v *Volume) withDirectAtomic(ctx context.Context, fn func(block.StoreOps, *block_gc.RefGraph) error) error {
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

	// Apply the direct operation.
	if fn != nil {
		if err := fn(blocks, rg); err != nil {
			return err
		}
	}

	// Carry the deferred edits in a changing transaction only, and end an
	// unchanged one without a commit.
	var edits deferredEdits
	if fn == nil || store.Wrote() {
		edits, err = v.applyDeferred(ctx, blocks, rg)
	}
	committed := false
	if err == nil && store.Wrote() {
		// Commit using the Volume write-ordering contract.
		if v.ordered != nil {
			err = kvtx.CommitOrdered(ctx, tx)
		} else {
			err = tx.Commit(ctx)
		}
		committed = err == nil
	}

	// Settle the deferred edits and wake Volume statistics watchers.
	v.settleDeferred(edits, committed)
	if committed {
		v.broadcastStorageStatsChanged()
	}
	return err
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
	return v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
		// Write unowned blocks directly in the shared transaction.
		if owner == "" {
			return put(blocks)
		}

		// Claim the owner, then write the blocks and flush their ownership
		// together.
		if err := claim(ctx, rg, owner); err != nil {
			return err
		}
		gc := block_gc.NewGCStoreOpsWithParentAndTraceTask(blocks, rg, owner, block_gc.BucketFlushTask())
		if err := put(gc); err != nil {
			return err
		}
		return gc.FlushPending(ctx)
	})
}

// sweepChunkSize bounds the candidates one reference batch releases. The
// orphan marker owns every candidate, so releasing nodes one at a time would
// rewrite its posting list once per node, which is quadratic in the number of
// orphans.
const sweepChunkSize = 64

// sweepBudget bounds the time one sweep transaction spends releasing chunks
// while it holds the writer. A commit costs a write barrier however little it
// changes, so a transaction sweeps chunks until the budget runs out.
const sweepBudget = 50 * time.Millisecond

// SweepUnreferenced treats candidate snapshots as hints, not deletion authority.
// Current owners, outgoing edges, orphan markers, and physical deletion of each
// candidate share one raw transaction with the same serialization as grouped
// head publication. Each transaction sweeps chunks of candidates until
// sweepBudget elapses. It returns the nodes removed by committed transactions,
// also when a later transaction fails.
func (v *Volume) SweepUnreferenced(ctx context.Context, graph block_gc.RefGraphOps, nodes []string) ([]string, error) {
	// Require the Volume transaction domain for atomic sweeping.
	actual, ok := v.refGraph.(*transactionRefGraph)
	given, sameType := graph.(*transactionRefGraph)
	if !ok || !sameType || given != actual || !v.SupportsAtomicPublication() {
		return nil, block_gc.ErrAtomicSweepUnsupported
	}

	// Sweep the candidates in budgeted transactions. Each consumes chunks from
	// the front of nodes.
	var swept []string
	for len(nodes) != 0 {
		var batch []string
		err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
			start := time.Now()
			for len(nodes) != 0 && time.Since(start) < sweepBudget {
				chunk := nodes[:min(len(nodes), sweepChunkSize)]
				nodes = nodes[len(chunk):]
				removed, err := sweepChunk(ctx, blocks, rg, chunk)
				if err != nil {
					return err
				}
				batch = append(batch, removed...)
			}
			return nil
		})

		// Forget the pending proofs of the nodes the transaction may have
		// removed, and report only the nodes of committed transactions.
		v.forgetSweptProofs(batch)
		if err != nil {
			return swept, err
		}
		swept = append(swept, batch...)
	}
	return swept, nil
}

// sweepChunk removes the chunk's candidates the orphan marker still owns alone,
// with their edges and blocks, and returns them.
func sweepChunk(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, chunk []string) ([]string, error) {
	// Collect the candidates the marker still owns alone, with their edges.
	var swept []string
	var removes []block_gc.RefEdge
	seen := make(map[string]struct{}, len(chunk))
	for _, node := range chunk {
		if _, dup := seen[node]; dup {
			continue
		}
		seen[node] = struct{}{}
		orphan, err := isMarkedOrphan(ctx, rg, node)
		if err != nil {
			return nil, err
		}
		if !orphan {
			continue
		}
		targets, err := rg.GetOutgoingRefs(ctx, node)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			removes = append(removes, block_gc.RefEdge{Subject: node, Object: target})
		}
		removes = append(removes, block_gc.RefEdge{Subject: block_gc.NodeUnreferenced, Object: node})
		swept = append(swept, node)
	}
	if len(swept) == 0 {
		return nil, nil
	}

	// Release the edges together. Removing a node's own marker keeps it
	// unmarked, and each child left without an owner gains a marker.
	if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
		return nil, err
	}

	// Delete the swept blocks.
	for _, node := range swept {
		if ref, ok := block_gc.ParseBlockIRI(node); ok {
			if err := blocks.RmBlock(ctx, ref); err != nil {
				return nil, err
			}
		}
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
