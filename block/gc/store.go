package block_gc

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// pendingRef is a buffered ref graph operation.
type pendingRef struct {
	source, target string
}

// GCStoreOps wraps a StoreOps with GC ref graph tracking.
//
// PutBlock and RecordBlockRefs are called from Transaction.Write's
// concurrent worker goroutines. Since the RefGraph shares the block
// cursor's mutex, writing to the RefGraph inside those goroutines
// would deadlock. Instead, GCStoreOps buffers the operations and
// they are flushed via FlushPending after Transaction.Write returns.
type GCStoreOps struct {
	store    block.StoreOps
	refGraph *RefGraph

	mu             sync.Mutex
	pendingUnref   []string     // block IRIs needing unreferenced -> block edges
	pendingRefs    []pendingRef // source -> target block ref edges
	pendingUnunref []string     // block IRIs to remove from unreferenced
}

// NewGCStoreOps wraps a StoreOps with GC ref graph tracking.
func NewGCStoreOps(store block.StoreOps, refGraph *RefGraph) *GCStoreOps {
	return &GCStoreOps{
		store:    store,
		refGraph: refGraph,
	}
}

// GetHashType returns the preferred hash type for the store.
func (g *GCStoreOps) GetHashType() hash.HashType {
	return g.store.GetHashType()
}

// PutBlock puts a block into the store and buffers an unreferenced
// gc/ref edge for later flush if the block is new.
func (g *GCStoreOps) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := g.store.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	if !existed && ref != nil && !ref.GetEmpty() {
		iri := BlockIRI(ref)
		g.mu.Lock()
		g.pendingUnref = append(g.pendingUnref, iri)
		g.mu.Unlock()
	}
	return ref, existed, nil
}

// GetBlock gets a block with the given reference.
func (g *GCStoreOps) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return g.store.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists.
func (g *GCStoreOps) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return g.store.GetBlockExists(ctx, ref)
}

// RmBlock cleans up the ref graph for a block without performing a
// physical delete. The Collector handles physical deletion. This
// removes all outgoing gc/ref edges from the block, removes the
// unreferenced -> block edge, and cascades orphan detection to any
// targets that lost their last incoming reference.
func (g *GCStoreOps) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	iri := BlockIRI(ref)

	if _, err := g.refGraph.RemoveNodeRefs(ctx, iri, true); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "remove outgoing refs")
	}

	return g.refGraph.RemoveRef(ctx, NodeUnreferenced, iri)
}

// RecordBlockRefs buffers block-to-block reference edges for later flush.
func (g *GCStoreOps) RecordBlockRefs(_ context.Context, source *block.BlockRef, targets []*block.BlockRef) error {
	sourceIRI := BlockIRI(source)
	g.mu.Lock()
	for _, t := range targets {
		if t == nil || t.GetEmpty() {
			continue
		}
		targetIRI := BlockIRI(t)
		g.pendingRefs = append(g.pendingRefs, pendingRef{sourceIRI, targetIRI})
		g.pendingUnunref = append(g.pendingUnunref, targetIRI)
	}
	g.mu.Unlock()
	return nil
}

// FlushPending writes all buffered PutBlock and RecordBlockRefs
// operations to the RefGraph. Must be called after Transaction.Write
// completes and the cursor mutex is no longer held.
func (g *GCStoreOps) FlushPending(ctx context.Context) error {
	g.mu.Lock()
	unrefs := g.pendingUnref
	refs := g.pendingRefs
	ununrefs := g.pendingUnunref
	g.pendingUnref = nil
	g.pendingRefs = nil
	g.pendingUnunref = nil
	g.mu.Unlock()

	for _, iri := range unrefs {
		if err := g.refGraph.AddRef(ctx, NodeUnreferenced, iri); err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			return errors.Wrap(err, "flush unreferenced edge")
		}
	}
	for _, r := range refs {
		if err := g.refGraph.AddRef(ctx, r.source, r.target); err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			return errors.Wrap(err, "flush block ref")
		}
	}
	for _, iri := range ununrefs {
		if err := g.refGraph.RemoveRef(ctx, NodeUnreferenced, iri); err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			return errors.Wrap(err, "flush remove unreferenced edge")
		}
	}
	return nil
}

// AddGCRef adds a gc/ref edge from subject to object and removes
// the unreferenced edge from the object (it now has a real reference).
func (g *GCStoreOps) AddGCRef(ctx context.Context, subject, object string) error {
	if err := g.refGraph.AddRef(ctx, subject, object); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "add gc ref")
	}
	return g.refGraph.RemoveRef(ctx, NodeUnreferenced, object)
}

// RemoveGCRef removes a gc/ref edge from subject to object and marks
// the object as orphaned if it has no remaining incoming references.
func (g *GCStoreOps) RemoveGCRef(ctx context.Context, subject, object string) error {
	if err := g.refGraph.RemoveRef(ctx, subject, object); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "remove gc ref")
	}
	if IsPermanentRoot(object) {
		return nil
	}
	has, err := g.refGraph.HasIncomingRefs(ctx, object)
	if err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "check incoming refs")
	}
	if !has {
		return g.refGraph.AddRef(ctx, NodeUnreferenced, object)
	}
	return nil
}

// GetRefGraph returns the underlying ref graph.
func (g *GCStoreOps) GetRefGraph() *RefGraph {
	return g.refGraph
}

// GetStore returns the underlying store.
func (g *GCStoreOps) GetStore() block.StoreOps {
	return g.store
}

// _ is a type assertion
var (
	_ block.StoreOps         = ((*GCStoreOps)(nil))
	_ block.BlockRefRecorder = ((*GCStoreOps)(nil))
)
