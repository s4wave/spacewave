package provider_local

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
)

// peerClassQueue bounds the served peer blocks waiting to be classified.
const peerClassQueue = 64

// peerClassWait bounds the lookups classifying one served peer block.
const peerClassWait = time.Second

// peerClass is what this node's backend knew of a block a peer served it.
type peerClass int

const (
	// peerUnclassified is a block left unclassified: the queue was full, a
	// lookup failed or outlived peerClassWait, or the placement has no open
	// backend whose index keeps its blocks.
	peerUnclassified peerClass = iota
	// peerNotIndexed is a block the backend's local index places nowhere.
	peerNotIndexed
	// peerIndexedMissing is a block the index places that the backend's packs
	// did not confirm.
	peerIndexedMissing
	// peerIndexedHeld is a block the index places and the packs confirm.
	peerIndexedHeld
	// peerClassCount is the number of classes.
	peerClassCount
)

// classify hands a block a peer served to the placement's classifier without
// waiting, counting it unclassified when the queue is full. A placement that
// classifies nothing, such as one in serve mode, has no queue.
func (c *peerReadCounts) classify(ref *block.BlockRef) {
	// Classify nothing without a queue.
	if c == nil || c.queue == nil {
		return
	}

	// Queue the block, or count it unclassified when the queue is full.
	select {
	case c.queue <- ref:
	default:
		c.classes[peerUnclassified].Add(1)
	}
}

// startPeerClassify starts the classifier of the blocks peers serve the
// placement, which runs until ctx ends. Serve mode classifies nothing.
func (p *placementState) startPeerClassify(ctx context.Context) {
	// Serve mode classifies nothing.
	if p.serveOnly {
		return
	}

	// Classify the queued blocks one at a time until ctx ends.
	queue := make(chan *block.BlockRef, peerClassQueue)
	p.peerReads.queue = queue
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ref := <-queue:
				p.peerReads.classes[p.classifyServed(ctx, ref)].Add(1)
			}
		}
	}()
}

// classifyServed classifies a block a peer served within peerClassWait.
func (p *placementState) classifyServed(ctx context.Context, ref *block.BlockRef) peerClass {
	// Look the block up within the bound, which leaves it unclassified.
	lookupCtx, cancel := context.WithTimeout(ctx, peerClassWait)
	defer cancel()
	class := p.locateBlock(lookupCtx, ref)
	if lookupCtx.Err() != nil {
		return peerUnclassified
	}
	return class
}

// locateBlock reports what the open backend of the current placement knows
// of the block: whether its local pack index places it, and whether the
// backend's existence check, which probes the packs' own indexes, confirms
// it. A placement with no open backend, one whose backend has no index or
// reclaims blocks, or a failed index lookup leaves it unclassified; a failed
// existence check counts as one that did not confirm it.
func (p *placementState) locateBlock(ctx context.Context, ref *block.BlockRef) peerClass {
	// Ask only the open backend of the current placement.
	if p.remote == nil {
		return peerUnclassified
	}
	remote := p.remote.Load()
	if remote == nil || !remote.backend.EqualVT(p.backend.Load()) {
		return peerUnclassified
	}
	locator, ok := remote.store.(block_store_writeback.BlockLocator)
	if !ok || !locator.KeepsBlocks() {
		return peerUnclassified
	}

	// Look the block up in the index.
	placed, err := locator.LocateBlocks(ctx, []*block.BlockRef{ref})
	if err != nil || len(placed) != 1 {
		return peerUnclassified
	}
	if !placed[0] {
		return peerNotIndexed
	}

	// Confirm the block in the backend's packs.
	held, err := remote.store.GetBlockExists(ctx, ref)
	if err != nil || !held {
		return peerIndexedMissing
	}
	return peerIndexedHeld
}
