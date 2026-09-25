package block_gc

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// GetStoredBlock reads a block and its outgoing block edges. The edges come
// from the ref graph, the journal edges it has not applied, and the edges this
// store has buffered but not flushed. Blocks are immutable, so an edge once
// recorded stays valid for the life of the block.
func (g *GCStoreOps) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, found, err := g.store.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	refs, err := g.getOutgoingBlockRefs(ctx, BlockIRI(ref))
	if err != nil {
		return nil, err
	}
	return &block.StoredBlock{Data: data, Refs: refs, RefsKnown: true}, nil
}

// getOutgoingBlockRefs collects the block targets of node's outgoing edges.
func (g *GCStoreOps) getOutgoingBlockRefs(ctx context.Context, node string) ([]*block.BlockRef, error) {
	targets, err := g.refGraph.GetOutgoingRefs(ctx, node)
	if err != nil {
		return nil, err
	}
	if g.wal != nil {
		journaled, err := g.wal.GetPendingOutgoingRefs(ctx, node)
		if err != nil {
			return nil, err
		}
		targets = append(targets, journaled...)
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
