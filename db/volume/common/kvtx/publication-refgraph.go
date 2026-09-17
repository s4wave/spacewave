package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// transactionRefGraph never carries Cayley IRI/quad caches across physical
// transactions. Atomic publication may remove/recreate nodes that a long-lived
// handle has cached; reusing that cache can resurrect obsolete IDs. Each public
// operation uses a fresh handle and one transaction in the publication domain.
// The durable prefix and graph representation are unchanged.
type transactionRefGraph struct{ volume *Volume }

// read runs fn against a fresh read-scoped reference graph in one transaction.
func (g *transactionRefGraph) read(ctx context.Context, fn func(*block_gc.RefGraph) error) error {
	v := g.volume
	v.directMu.RLock()
	defer v.directMu.RUnlock()
	if v.directClosed {
		return block.ErrPublicationClosed
	}
	tx, err := v.kvtxStore.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	rg, err := block_gc.NewRefGraph(ctx, kvtx.NewTxStore(tx), volumeRefGraphPrefix())
	if err != nil {
		return err
	}
	defer rg.Close()
	return fn(rg)
}

// write runs fn against a fresh writable reference graph in one transaction.
func (g *transactionRefGraph) write(ctx context.Context, fn func(*block_gc.RefGraph) error) error {
	return g.volume.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		err := fn(rg)
		return err == nil, err
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

// GetIncomingRefs returns the reference sources of a node.
func (g *transactionRefGraph) GetIncomingRefs(ctx context.Context, node string) (refs []string, err error) {
	err = g.read(ctx, func(rg *block_gc.RefGraph) error { refs, err = rg.GetIncomingRefs(ctx, node); return err })
	return
}

// GetUnreferencedNodes returns the nodes without any outgoing reference edge.
func (g *transactionRefGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return g.GetOutgoingRefs(ctx, block_gc.NodeUnreferenced)
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

var _ block_gc.RefGraphOps = (*transactionRefGraph)(nil)

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
