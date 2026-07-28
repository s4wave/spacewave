//go:build js

package gcgraph

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// ApplyRefBatch applies a ref graph ownership transition while holding the
// graph-wide ownership lock.
func (g *GCGraph) ApplyRefBatch(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	release, err := g.acquireOwnershipLock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return g.applyRefBatchLocked(ctx, adds, removes)
}

func (g *GCGraph) applyRefBatchLocked(
	ctx context.Context,
	adds, removes []block_gc.RefEdge,
) error {
	adds, removes, err := g.prepareRefBatch(ctx, adds, removes)
	if err != nil {
		return err
	}
	for _, e := range adds {
		if err := g.addRef(e.Subject, e.Object); err != nil {
			return err
		}
	}
	for _, e := range removes {
		if err := g.removeRef(e.Subject, e.Object); err != nil {
			return err
		}
	}
	return nil
}

func (g *GCGraph) prepareRefBatch(
	ctx context.Context,
	adds, removes []block_gc.RefEdge,
) ([]block_gc.RefEdge, []block_gc.RefEdge, error) {
	owners := make(map[string]map[string]struct{})
	stagingRemoves := make(map[string]struct{})
	for _, edge := range removes {
		if edge.Subject == block_gc.NodeUnreferenced {
			stagingRemoves[edge.Object] = struct{}{}
			continue
		}
		if block_gc.IsPermanentRoot(edge.Object) {
			continue
		}
		if _, ok := owners[edge.Object]; ok {
			continue
		}
		sources, err := g.GetIncomingRefs(ctx, edge.Object)
		if err != nil {
			return nil, nil, err
		}
		set := make(map[string]struct{}, len(sources))
		for _, source := range sources {
			if source != block_gc.NodeUnreferenced {
				set[source] = struct{}{}
			}
		}
		owners[edge.Object] = set
	}
	for _, edge := range removes {
		if set, ok := owners[edge.Object]; ok {
			delete(set, edge.Subject)
		}
	}
	for _, edge := range adds {
		if set, ok := owners[edge.Object]; ok && edge.Subject != block_gc.NodeUnreferenced {
			set[edge.Subject] = struct{}{}
		}
	}
	for object, set := range owners {
		if len(set) == 0 {
			if _, removingStaging := stagingRemoves[object]; !removingStaging {
				adds = append(adds, block_gc.RefEdge{
					Subject: block_gc.NodeUnreferenced,
					Object:  object,
				})
			}
		}
	}
	return adds, removes, nil
}

// RemoveNodeRefs removes all outgoing gc/ref edges for a node.
// Returns the list of target IRIs that lost an incoming edge.
// If markOrphaned is true, targets with no remaining incoming
// refs get an unreferenced edge.
func (g *GCGraph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error) {
	release, err := g.acquireOwnershipLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	targets, err := g.GetOutgoingRefs(ctx, node)
	if err != nil {
		return nil, err
	}
	removes := make([]block_gc.RefEdge, 0, len(targets))
	for _, target := range targets {
		removes = append(removes, block_gc.RefEdge{Subject: node, Object: target})
	}
	if markOrphaned {
		if err := g.applyRefBatchLocked(ctx, nil, removes); err != nil {
			return nil, err
		}
	} else {
		for _, edge := range removes {
			if err := g.removeRef(edge.Subject, edge.Object); err != nil {
				return nil, err
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
