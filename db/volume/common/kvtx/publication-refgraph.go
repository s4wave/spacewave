package kvtx

import (
	"context"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// transactionRefGraph runs each public operation in one physical transaction
// of the publication domain, joined with the Volume's direct operations so it
// cannot outlive Close.
type transactionRefGraph struct{ volume *Volume }

// read runs fn against a fresh read-scoped reference graph in one transaction.
func (g *transactionRefGraph) read(ctx context.Context, fn func(*block_gc.RefGraph) error) error {
	// Join Volume operations while reading the reference graph.
	v := g.volume
	v.directMu.RLock()
	defer v.directMu.RUnlock()
	if v.directClosed {
		return block.ErrPublicationClosed
	}

	// Open a read transaction for the reference graph snapshot.
	tx, err := v.kvtxStore.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Read through a fresh reference graph scoped to the transaction.
	return fn(OpenRefGraph(kvtx.NewTxStore(tx)))
}

// write runs fn against a fresh writable reference graph in one transaction.
func (g *transactionRefGraph) write(ctx context.Context, fn func(*block_gc.RefGraph) error) error {
	return g.volume.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
		return fn(rg)
	})
}

// AddRef adds a reference edge between two graph nodes.
func (g *transactionRefGraph) AddRef(ctx context.Context, subject, object string) error {
	return g.write(ctx, func(rg *block_gc.RefGraph) error { return rg.AddRef(ctx, subject, object) })
}

// RemoveRef removes a reference edge between two graph nodes.
func (g *transactionRefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	return g.write(ctx, func(rg *block_gc.RefGraph) error { return rg.RemoveRef(ctx, subject, object) })
}

// ApplyRefBatch applies reference additions and removals in one transaction.
func (g *transactionRefGraph) ApplyRefBatch(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	err := g.write(ctx, func(rg *block_gc.RefGraph) error { return rg.ApplyRefBatch(ctx, adds, removes) })
	if err != nil {
		return &atomicRefBatchError{err: err, adds: append([]block_gc.RefEdge(nil), adds...), removes: append([]block_gc.RefEdge(nil), removes...)}
	}
	return nil
}

// RemoveNodeRefs removes every reference edge of a node, optionally marking it
// orphaned, and returns the former targets.
func (g *transactionRefGraph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) (targets []string, err error) {
	err = g.write(ctx, func(rg *block_gc.RefGraph) error {
		targets, err = rg.RemoveNodeRefs(ctx, node, markOrphaned)
		return err
	})
	return
}

// HasIncomingRefs reports whether a node has any incoming reference edge.
func (g *transactionRefGraph) HasIncomingRefs(ctx context.Context, node string) (found bool, err error) {
	return g.HasIncomingRefsExcluding(ctx, node)
}

// HasIncomingRefsExcluding reports whether a node has an incoming reference
// edge other than the excluded subjects.
func (g *transactionRefGraph) HasIncomingRefsExcluding(ctx context.Context, node string, excluded ...string) (found bool, err error) {
	err = g.read(ctx, func(rg *block_gc.RefGraph) error {
		found, err = rg.HasIncomingRefsExcluding(ctx, node, excluded...)
		return err
	})
	return
}

// GetOutgoingRefs returns the reference targets of a node.
func (g *transactionRefGraph) GetOutgoingRefs(ctx context.Context, node string) (refs []string, err error) {
	err = g.read(ctx, func(rg *block_gc.RefGraph) error { refs, err = rg.GetOutgoingRefs(ctx, node); return err })
	return
}

// GetBlockEdges returns a block's outgoing edges and whether the graph tracks
// it, in one read transaction.
func (g *transactionRefGraph) GetBlockEdges(ctx context.Context, node string) (targets []string, tracked bool, err error) {
	err = g.read(ctx, func(rg *block_gc.RefGraph) error { targets, tracked, err = rg.GetBlockEdges(ctx, node); return err })
	return
}

// GetIncomingRefs returns the reference sources of a node.
func (g *transactionRefGraph) GetIncomingRefs(ctx context.Context, node string) (refs []string, err error) {
	err = g.read(ctx, func(rg *block_gc.RefGraph) error { refs, err = rg.GetIncomingRefs(ctx, node); return err })
	return
}

// GetUnreferencedNodes returns the nodes without any outgoing reference edge.
func (g *transactionRefGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return g.GetOutgoingRefs(ctx, block_gc.NodeUnreferenced)
}

// TrackedBlocks reports which blocks the graph tracks in one read snapshot.
func (g *transactionRefGraph) TrackedBlocks(ctx context.Context, refs []*block.BlockRef) (tracked []bool, err error) {
	err = g.read(ctx, func(rg *block_gc.RefGraph) error { tracked, err = rg.TrackedBlocks(ctx, refs); return err })
	return
}

// MarkBlocks marks the graph's blocks in one read snapshot.
func (g *transactionRefGraph) MarkBlocks(ctx context.Context, opts ...block_gc.MarkOption) (mark *block_gc.BlockMark, err error) {
	err = g.read(ctx, func(rg *block_gc.RefGraph) error { mark, err = rg.MarkBlocks(ctx, opts...); return err })
	return
}

// TraceOwners finds the owned path to each block, as RefGraph.TraceOwners
// describes, reading each level in its own transaction.
func (g *transactionRefGraph) TraceOwners(ctx context.Context, blocks []string, maxDepth int) ([]block_gc.OwnerTrace, error) {
	return OpenRefGraph(&directReadStore{volume: g.volume}).TraceOwners(ctx, blocks, maxDepth)
}

// MarkBlocksScan marks the graph's blocks by scanning child lists in short
// read transactions, as RefGraph.MarkBlocksScan describes, settling the holes
// the Volume does not hold. Each transaction joins the Volume's direct
// operations on its own, so neither a snapshot nor Close waits on the whole
// mark.
func (g *transactionRefGraph) MarkBlocksScan(ctx context.Context, opts ...block_gc.MarkOption) (*block_gc.BlockMark, error) {
	return OpenRefGraph(&directReadStore{volume: g.volume}).MarkBlocksScan(ctx, g.volume.blocksStored, opts...)
}

// storedProbeBatch bounds the blocks one read transaction of blocksStored
// checks, so each stays short.
const storedProbeBatch = 512

// blocksStored reports which block nodes the Volume holds, counting the blocks
// of admitted publications, in read transactions of at most storedProbeBatch
// blocks each.
func (v *Volume) blocksStored(ctx context.Context, nodes []string) ([]bool, error) {
	found := make([]bool, 0, len(nodes))
	for chunk := range slices.Chunk(nodes, storedProbeBatch) {
		// Parse the chunk; a node that is not a block counts as held.
		refs := make([]*block.BlockRef, 0, len(chunk))
		at := make([]int, 0, len(chunk))
		held := make([]bool, len(chunk))
		for i, node := range chunk {
			ref, ok := block_gc.ParseBlockIRI(node)
			if !ok {
				held[i] = true
				continue
			}
			refs = append(refs, ref)
			at = append(at, i)
		}

		// Check the blocks in one read.
		exists, err := v.GetBlockExistsBatch(ctx, refs)
		if err != nil {
			return nil, err
		}
		for j, ok := range exists {
			held[at[j]] = ok
		}
		found = append(found, held...)
	}
	return found, nil
}

// directReadStore opens read transactions of the Volume's store, each joined
// with the Volume's direct operations until it is discarded.
type directReadStore struct{ volume *Volume }

// NewTransaction opens a read transaction holding directMu's read lock.
func (s *directReadStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Join Volume operations, unless closed.
	if write {
		return nil, errMarkReadOnly
	}
	v := s.volume
	v.directMu.RLock()
	if v.directClosed {
		v.directMu.RUnlock()
		return nil, block.ErrPublicationClosed
	}

	// Open the transaction, releasing the lock with it.
	tx, err := v.kvtxStore.NewTransaction(ctx, false)
	if err != nil {
		v.directMu.RUnlock()
		return nil, err
	}
	return &directReadTx{Tx: tx, release: sync.OnceFunc(v.directMu.RUnlock)}, nil
}

// directReadTx releases directMu's read lock when it ends.
type directReadTx struct {
	kvtx.Tx
	// release unlocks directMu once.
	release func()
}

// Commit ends the transaction and releases the lock.
func (t *directReadTx) Commit(ctx context.Context) error {
	defer t.release()
	return t.Tx.Commit(ctx)
}

// Discard ends the transaction and releases the lock.
func (t *directReadTx) Discard() {
	t.Tx.Discard()
	t.release()
}

// AddBlockRef adds a block-to-block reference edge.
func (g *transactionRefGraph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	return g.write(ctx, func(rg *block_gc.RefGraph) error { return rg.AddBlockRef(ctx, source, target) })
}

// AddObjectRoot adds an object root reference edge.
func (g *transactionRefGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	return g.write(ctx, func(rg *block_gc.RefGraph) error { return rg.AddObjectRoot(ctx, objectKey, ref) })
}

// RemoveObjectRoot removes an object root reference edge.
func (g *transactionRefGraph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	return g.write(ctx, func(rg *block_gc.RefGraph) error { return rg.RemoveObjectRoot(ctx, objectKey, ref) })
}

// Close releases the transaction-scoped graph; it holds no resources.
func (*transactionRefGraph) Close() error { return nil }

// The nested TxStore commits are virtual. If a later slice or the outer commit
// fails, none of the transition became durable; never expose a suffix-only
// remainder from RefGraph.ApplyRefBatch to a caller that retains retry work.
type atomicRefBatchError struct {
	// err is the underlying physical failure.
	err error
	// adds are the reference additions attempted in the failed batch.
	adds []block_gc.RefEdge
	// removes are the reference removals attempted in the failed batch.
	removes []block_gc.RefEdge
}

// Error returns the underlying physical failure message.
func (e *atomicRefBatchError) Error() string { return e.err.Error() }

// Unwrap returns the underlying physical failure.
func (e *atomicRefBatchError) Unwrap() error { return e.err }

// RefBatchRemainder returns cloned copies of the attempted additions and removals.
func (e *atomicRefBatchError) RefBatchRemainder() ([]block_gc.RefEdge, []block_gc.RefEdge) {
	return append([]block_gc.RefEdge(nil), e.adds...), append([]block_gc.RefEdge(nil), e.removes...)
}

// _ is a type assertion
var (
	_ block_gc.RefGraphOps     = (*transactionRefGraph)(nil)
	_ block_gc.BlockEdgeReader = (*transactionRefGraph)(nil)
)

// _ is a type assertion.
var (
	_ block_gc.BlockMarker  = (*transactionRefGraph)(nil)
	_ block_gc.BlockTracker = (*transactionRefGraph)(nil)
)
