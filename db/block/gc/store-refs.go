package block_gc

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// GetStoredBlock reads a block and its outgoing block edges. The edges come
// from the ref graph and the edges this store has buffered but not flushed.
// Blocks are immutable, so an edge once recorded stays valid for the life of
// the block. Without outgoing refs or an incoming edge, the graph cannot
// distinguish a leaf from an untracked block.
func (g *GCStoreOps) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read the immutable block data before resolving its outgoing references.
	data, found, err := g.store.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	stored, err := g.getBlockEdges(ctx, ref, data)
	if err != nil || stored.RefsKnown {
		return stored, err
	}

	// A flush takes the buffered edges before it delivers them, so a block it
	// is tracking can read as untracked. Read again between flushes.
	release, err := g.flushMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return g.getBlockEdges(ctx, ref, data)
}

// getBlockEdges resolves the outgoing edges of a stored block.
func (g *GCStoreOps) getBlockEdges(ctx context.Context, ref *block.BlockRef, data []byte) (*block.StoredBlock, error) {
	// Collect the recorded and buffered references of the stored block.
	node := BlockIRI(ref)
	refs, err := g.getOutgoingBlockRefs(ctx, node)
	if err != nil {
		return nil, err
	}

	// A re-put carries the block's own refs before FlushPending restores them.
	if len(refs) == 0 {
		g.mu.Lock()
		for _, reput := range g.pendingReputs {
			if reput.ref.EqualsRef(ref) {
				refs = block.CloneBlockRefs(reput.targets)
				break
			}
		}
		g.mu.Unlock()
	}

	// Distinguish a tracked leaf from bytes whose edges were never recorded.
	// The graph reports the staging edge of a block that has a child list; an
	// owner alone does not make the list, so a block taken by a parent after
	// an untracked write stays unknown rather than read as a leaf.
	known := len(refs) != 0 || g.hasBufferedBlockEdges(node)
	if !known {
		incoming, err := g.refGraph.GetIncomingRefs(ctx, node)
		if err != nil {
			return nil, err
		}
		known = slices.Contains(incoming, NodeUnreferenced)
	}
	return &block.StoredBlock{Data: data, Refs: refs, RefsKnown: known}, nil
}

// hasBufferedBlockEdges reports whether node participates in a buffered edge,
// including staging and ownership edges not yet materialized by FlushPending.
func (g *GCStoreOps) hasBufferedBlockEdges(node string) bool {
	// Check staging and ownership buffered by block writes.
	g.mu.Lock()
	defer g.mu.Unlock()
	if slices.Contains(g.pendingUnref, node) || slices.Contains(g.pendingStaged, node) {
		return true
	}

	// Check child edges and normalized additions retained after a failed flush.
	for _, edge := range g.pendingRefs {
		if edge.source == node || edge.target == node {
			return true
		}
	}
	for _, edge := range g.pendingAdds {
		if edge.Subject == node || edge.Object == node {
			return true
		}
	}
	return false
}

// getOutgoingBlockRefs collects the block targets of node's outgoing edges.
func (g *GCStoreOps) getOutgoingBlockRefs(ctx context.Context, node string) ([]*block.BlockRef, error) {
	// Read the recorded outgoing edges.
	targets, err := g.refGraph.GetOutgoingRefs(ctx, node)
	if err != nil {
		return nil, err
	}

	// Include edges buffered for the next flush, or left by a failed flush.
	g.mu.Lock()
	for _, edge := range g.pendingRefs {
		if edge.source == node {
			targets = append(targets, edge.target)
		}
	}
	for _, edge := range g.pendingAdds {
		if edge.Subject == node {
			targets = append(targets, edge.Object)
		}
	}
	g.mu.Unlock()

	// Keep one ref per block target; other nodes are not block edges.
	slices.Sort(targets)
	targets = slices.Compact(targets)
	refs := make([]*block.BlockRef, 0, len(targets))
	for _, target := range targets {
		if ref, ok := ParseBlockIRI(target); ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
