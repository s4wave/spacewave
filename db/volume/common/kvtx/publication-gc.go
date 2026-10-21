package kvtx

import (
	"context"
	"errors"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
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
	p, err := v.openPhysical(ctx)
	if err != nil {
		return err
	}
	defer p.tx.Discard()

	// Apply the direct operation.
	if fn != nil {
		if err := fn(p.blocks, p.rg); err != nil {
			return err
		}
	}

	// Carry the deferred edits in a changing transaction only, and end an
	// unchanged one without a commit.
	var edits deferredEdits
	if fn == nil || p.wrote() {
		edits, err = v.applyDeferred(ctx, p.blocks, p.rg)
	}
	committed := false
	if err == nil && p.wrote() {
		// Commit using the Volume write-ordering contract.
		if v.ordered != nil {
			err = kvtx.CommitOrdered(ctx, p.tx)
		} else {
			err = p.tx.Commit(ctx)
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
func (v *Volume) PrepareOwnedBlockBatch(ctx context.Context, bucketID string, entries []*block.PutBatchEntry) (existed []bool, err error) {
	if len(entries) == 0 && v.SupportsAtomicPublication() {
		return nil, nil
	}
	err = v.prepareOwned(ctx, bucketOwner(bucketID), claimBucket, func(store block.StoreOps) error {
		existed, err = store.PutBlockBatch(ctx, entries)
		return err
	})
	return existed, err
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

// sweepChunkTime is the time one chunk should take. The budget is checked
// between chunks only, so chunks are sized to fit it: a process starved of CPU
// that overruns this time halves the next chunk instead of holding the writer
// for many budgets.
const sweepChunkTime = sweepBudget / 4

// SweepUnreferenced treats candidate snapshots as hints, not deletion authority.
// Current owners, outgoing edges, orphan markers, and physical deletion of each
// candidate share one raw transaction with the same serialization as grouped
// head publication. Each transaction sweeps chunks of candidates until
// sweepBudget elapses or another writer queues for the store, sizing each
// chunk by how long the last one took, so a foreground commit waits for at
// most one chunk. It returns the nodes removed by committed transactions, also
// when a later transaction fails.
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
	size := sweepChunkSize
	for len(nodes) != 0 {
		var batch []string
		err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
			start := time.Now()
			for len(nodes) != 0 {
				chunkStart := time.Now()
				chunk := nodes[:min(len(nodes), size)]
				nodes = nodes[len(chunk):]
				removed, err := sweepChunk(ctx, blocks, rg, chunk)
				if err != nil {
					return err
				}
				batch = append(batch, removed...)
				size = nextChunkSize(size, time.Since(chunkStart))
				if time.Since(start) >= sweepBudget || v.writersWaiting() {
					break
				}
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

// SweepUnreachedBlocks runs one block pass at a time per Volume. It opens the
// pass in its own commit, so each later write logs the blocks it adopts, then
// marks in a later snapshot without holding the writer. Each sweep transaction
// first rescues the logged blocks and every node they reach, then removes
// chunks of the unreached blocks still unrescued, with their records, sized
// and budgeted as SweepUnreferenced's are. A block a write adopts after the
// mark is therefore never removed: the write either committed before the
// transaction, which then rescues it, or waits behind it.
func (v *Volume) SweepUnreachedBlocks(ctx context.Context, graph block_gc.RefGraphOps, stats *block_gc.Stats) error {
	// Require the Volume transaction domain and run one pass at a time.
	actual, ok := v.refGraph.(*transactionRefGraph)
	given, sameType := graph.(*transactionRefGraph)
	if !ok || !sameType || given != actual || !v.SupportsAtomicPublication() {
		return block_gc.ErrAtomicSweepUnsupported
	}
	release, err := v.blockPassMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Open the pass, and close it when the pass ends, also on cancellation.
	if err := actual.write(ctx, func(rg *block_gc.RefGraph) error { return rg.BeginBlockPass(ctx) }); err != nil {
		return err
	}
	err = v.sweepBlockPass(ctx, actual, stats)
	endCtx := context.WithoutCancel(ctx)
	endErr := actual.write(endCtx, func(rg *block_gc.RefGraph) error { return rg.EndBlockPass(endCtx) })
	return errors.Join(err, endErr)
}

// sweepBlockPass marks the blocks of an open pass and sweeps the unreached.
func (v *Volume) sweepBlockPass(ctx context.Context, actual *transactionRefGraph, stats *block_gc.Stats) error {
	// Mark the blocks. A block of unknown references may name any
	// unreached block, so the pass then sweeps nothing.
	start := time.Now()
	mark, err := actual.MarkBlocks(ctx)
	stats.MarkDuration += time.Since(start)
	if err != nil {
		return err
	}
	stats.RecordMark(mark)
	if mark.Unknown != 0 {
		return nil
	}
	return v.sweepUnreached(ctx, mark, stats)
}

// sweepUnreached removes all the blocks mark did not reach in budgeted
// transactions, each first rescuing the blocks writes adopted since the last.
// Each transaction yields the writer after its budget or to a waiting writer,
// and the next continues the same list.
func (v *Volume) sweepUnreached(ctx context.Context, mark *block_gc.BlockMark, stats *block_gc.Stats) error {
	// Time the sweep, starting from the default chunk size.
	nodes := mark.Unreached
	size := sweepChunkSize
	start := time.Now()
	defer func() { stats.AtomicSweepDuration += time.Since(start) }()
	for len(nodes) != 0 {
		var batch []string
		err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
			// Rescue the blocks adopted since the last transaction.
			rescued, err := rg.RescueAdopted(ctx, mark)
			if err != nil {
				return err
			}
			stats.RescuedNodes += rescued

			// A rescued block of unknown references may name any unreached
			// block, so report it and stop sweeping.
			if mark.Unknown != 0 {
				stats.UnknownBlocks, stats.UnknownSample = mark.Unknown, mark.UnknownSample
				nodes = nil
				return nil
			}

			// Sweep chunks until the budget runs out or a writer waits.
			txStart := time.Now()
			for len(nodes) != 0 {
				chunkStart := time.Now()
				chunk := nodes[:min(len(nodes), size)]
				nodes = nodes[len(chunk):]
				stats.AtomicSweepCount += len(chunk)
				removed, err := sweepBlockChunk(ctx, blocks, rg, mark.Live, chunk)
				if err != nil {
					return err
				}
				batch = append(batch, removed...)
				size = nextChunkSize(size, time.Since(chunkStart))
				if time.Since(txStart) >= sweepBudget || v.writersWaiting() {
					break
				}
			}
			return nil
		})

		// Forget the pending proofs of the blocks the transaction may have
		// removed, and count only the blocks of committed transactions.
		v.forgetSweptProofs(batch)
		if err != nil {
			return err
		}
		stats.NodesSwept += len(batch)
		stats.BlocksSwept += len(batch)
		stats.RemoveNodeRefsCount += len(batch)
		stats.RemoveBlockCount += len(batch)
	}
	return nil
}

// sweepBlockChunk removes the chunk's blocks that the mark and the rescues
// did not reach, with their records, and returns them.
func sweepBlockChunk(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, live *block_gc.NodeSet, chunk []string) ([]string, error) {
	var swept []string
	for _, node := range chunk {
		ref, ok := block_gc.ParseBlockIRI(node)
		if !ok || live.Has(node) {
			continue
		}
		if _, err := rg.RemoveNodeRefs(ctx, node, true); err != nil {
			return nil, err
		}
		if err := blocks.RmBlock(ctx, ref); err != nil {
			return nil, err
		}
		swept = append(swept, node)
	}
	return swept, nil
}

// writersWaiting reports whether another writer queues for the store, so a
// sweep transaction commits early and lets it in.
func (v *Volume) writersWaiting() bool {
	q, ok := v.kvtxStore.(kvtx.WriterQueueStore)
	return ok && q.WritersWaiting()
}

// nextChunkSize sizes the chunk after one of size candidates that took took:
// half as large when it overran sweepChunkTime, and twice as large, up to
// sweepChunkSize, when it took under half of it.
func nextChunkSize(size int, took time.Duration) int {
	switch {
	case took > sweepChunkTime:
		return max(size/2, 1)
	case took < sweepChunkTime/2:
		return min(size*2, sweepChunkSize)
	}
	return size
}

// sweepChunk removes the chunk's candidates the orphan marker still owns alone,
// with their edges and blocks, and returns them. It drops the marker of each
// candidate another owner took since it was marked: the marker would
// otherwise list the node as a candidate in every later sweep, and the graph
// marks it again when it loses its last owner.
func sweepChunk(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, chunk []string) ([]string, error) {
	// Collect the candidates the marker still owns alone, with their edges,
	// and the candidates another owner took.
	var swept, taken []string
	var removes []block_gc.RefEdge
	released := make(map[string]struct{})
	seen := make(map[string]struct{}, len(chunk))
	for _, node := range chunk {
		if _, dup := seen[node]; dup {
			continue
		}
		seen[node] = struct{}{}
		marked, owned, err := markerState(ctx, rg, node)
		if err != nil {
			return nil, err
		}
		if !marked {
			continue
		}
		if owned {
			taken = append(taken, node)
			continue
		}
		targets, err := rg.GetOutgoingRefs(ctx, node)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			removes = append(removes, block_gc.RefEdge{Subject: node, Object: target})
			released[target] = struct{}{}
		}
		removes = append(removes, block_gc.RefEdge{Subject: block_gc.NodeUnreferenced, Object: node})
		swept = append(swept, node)
	}

	// Drop the stale markers. A node a swept node releases keeps its marker:
	// removing it would also stop the batch from marking the node when that
	// release takes its last owner.
	for _, node := range taken {
		if _, ok := released[node]; !ok {
			removes = append(removes, block_gc.RefEdge{Subject: block_gc.NodeUnreferenced, Object: node})
		}
	}
	if len(removes) == 0 {
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

// markerState reports whether the unreferenced marker holds node and whether
// another owner does. A permanent root reports neither, so it is never swept.
func markerState(ctx context.Context, rg *block_gc.RefGraph, node string) (marked, owned bool, err error) {
	// A permanent root is never swept.
	if block_gc.IsPermanentRoot(node) {
		return false, false, nil
	}

	// Classify the current owners.
	incoming, err := rg.GetIncomingRefs(ctx, node)
	if err != nil {
		return false, false, err
	}
	for _, owner := range incoming {
		if owner == block_gc.NodeUnreferenced {
			marked = true
		} else {
			owned = true
		}
	}
	return marked, owned, nil
}

var (
	_ block_gc.AtomicSweepStore = (*Volume)(nil)
	_ block.AtomicBlockPreparer = (*Volume)(nil)
)
