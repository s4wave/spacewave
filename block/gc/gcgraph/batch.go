//go:build js

package gcgraph

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	block_gc "github.com/aperturerobotics/hydra/block/gc"
)

// ApplyRefBatch applies ref graph edge additions followed by removals.
func (g *GCGraph) ApplyRefBatch(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	for _, e := range adds {
		if err := g.AddRef(ctx, e.Subject, e.Object); err != nil {
			return err
		}
	}
	for _, e := range removes {
		if err := g.RemoveRef(ctx, e.Subject, e.Object); err != nil {
			return err
		}
	}
	return nil
}

// RemoveNodeRefs removes all outgoing gc/ref edges for a node.
// Returns the list of target IRIs that lost an incoming edge.
// If markOrphaned is true, targets with no remaining incoming
// refs get an unreferenced edge.
func (g *GCGraph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error) {
	targets, err := g.GetOutgoingRefs(ctx, node)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if err := g.RemoveRef(ctx, node, t); err != nil {
			return nil, err
		}
	}
	if markOrphaned {
		for _, t := range targets {
			if block_gc.IsPermanentRoot(t) {
				continue
			}
			has, err := g.HasIncomingRefs(ctx, t)
			if err != nil {
				return nil, err
			}
			if !has {
				if err := g.AddRef(ctx, block_gc.NodeUnreferenced, t); err != nil {
					return nil, err
				}
			}
		}
	}
	return targets, nil
}

// AddBlockRef adds gc/ref from source block to target block.
func (g *GCGraph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	s := block_gc.BlockIRI(source)
	t := block_gc.BlockIRI(target)
	if s == "" || t == "" {
		return nil
	}
	return g.AddRef(ctx, s, t)
}

// AddObjectRoot adds gc/ref from object:{key} to block.
func (g *GCGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	t := block_gc.BlockIRI(ref)
	if t == "" {
		return nil
	}
	return g.AddRef(ctx, block_gc.ObjectIRI(objectKey), t)
}

// RemoveObjectRoot removes gc/ref from object:{key} to block.
func (g *GCGraph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	t := block_gc.BlockIRI(ref)
	if t == "" {
		return nil
	}
	return g.RemoveRef(ctx, block_gc.ObjectIRI(objectKey), t)
}

// Close is a no-op for the OPFS-backed graph store.
func (g *GCGraph) Close() error {
	return nil
}
