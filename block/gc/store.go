package block_gc

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// GCStoreOps wraps a StoreOps with GC ref graph tracking.
//
// On PutBlock, new blocks are registered as unreferenced in the ref
// graph (unreferenced -> block gc/ref edge). When RecordBlockRefs is
// called (by Transaction.Write), the unreferenced edge is removed from
// each target that gains a real reference. RmBlock cleans up graph
// edges and cascades orphan detection. AddGCRef/RemoveGCRef manage
// arbitrary subject -> object gc/ref edges.
type GCStoreOps struct {
	store    block.StoreOps
	refGraph *RefGraph
	onSwept  func(ctx context.Context, iri string) error
}

// NewGCStoreOps wraps a StoreOps with GC ref graph tracking.
// The onSwept callback is optional; if non-nil it is called for each
// node swept during orphan cascade (before physical delete).
func NewGCStoreOps(
	store block.StoreOps,
	refGraph *RefGraph,
	onSwept func(context.Context, string) error,
) *GCStoreOps {
	return &GCStoreOps{
		store:    store,
		refGraph: refGraph,
		onSwept:  onSwept,
	}
}

// GetHashType returns the preferred hash type for the store.
func (g *GCStoreOps) GetHashType() hash.HashType {
	return g.store.GetHashType()
}

// PutBlock puts a block into the store and records an unreferenced
// gc/ref edge in the ref graph if the block is new.
func (g *GCStoreOps) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := g.store.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	if !existed && ref != nil && !ref.GetEmpty() {
		if err := g.refGraph.AddRef(ctx, NodeUnreferenced, BlockIRI(ref)); err != nil {
			if ctx.Err() != nil {
				return ref, existed, context.Canceled
			}
			return ref, existed, errors.Wrap(err, "record unreferenced edge")
		}
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

// RecordBlockRefs records block-to-block reference edges and removes
// the unreferenced edge from each target that gains a reference.
func (g *GCStoreOps) RecordBlockRefs(ctx context.Context, source *block.BlockRef, targets []*block.BlockRef) error {
	sourceIRI := BlockIRI(source)
	for _, t := range targets {
		if t == nil || t.GetEmpty() {
			continue
		}
		targetIRI := BlockIRI(t)
		if err := g.refGraph.AddRef(ctx, sourceIRI, targetIRI); err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			return errors.Wrap(err, "add block ref")
		}
		if err := g.refGraph.RemoveRef(ctx, NodeUnreferenced, targetIRI); err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			return errors.Wrap(err, "remove unreferenced edge from target")
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
