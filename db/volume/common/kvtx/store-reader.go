package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// storeReader reads and writes a Volume's blocks in its store and reports
// what the decoded-block cache needs: a block the store serves is durable,
// since the store is the Volume's committed store, and every removal from it
// reports itself (RmBlock, tombstones, and physicalTx). The calls it does not
// override go to the store as they are.
type storeReader struct {
	// StoreOps reads and writes the applied blocks.
	block.StoreOps
	// volume serves the child lists of stored blocks.
	volume *Volume
	// scoped is set in a read scope, which reads one store snapshot.
	scoped bool
	// gen is the removal generation recorded before a read scope's snapshot
	// opened. Outside a read scope each read records its own.
	gen uint64
}

// BeginReadOperation opens a read scope on the store, recording the removal
// generation before its snapshot opens.
func (r storeReader) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// A read scope reads its snapshot for its life.
	if r.scoped {
		return r, func() {}, nil
	}

	// Record the removal generation, then open the snapshot.
	gen := block.DurableReadGeneration()
	scope, release, err := r.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return storeReader{StoreOps: scope, volume: r.volume, scoped: true, gen: gen}, release, nil
}

// PutBlockBatch writes a batch of blocks to the store, reporting its
// tombstones as removals.
func (r storeReader) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	defer block.InvalidateRemovedTombstones(entries)
	return r.StoreOps.PutBlockBatch(ctx, entries)
}

// readGen returns the removal generation for a read about to open: the read
// scope's, or the current one outside a scope.
func (r storeReader) readGen() uint64 {
	if r.scoped {
		return r.gen
	}
	return block.DurableReadGeneration()
}

// GetBlock reads the store and marks a found block durable.
func (r storeReader) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	gen := r.readGen()
	data, found, err := r.StoreOps.GetBlock(ctx, ref)
	if err == nil && found {
		block.MarkDurableRead(ctx, ref, gen)
	}
	return data, found, err
}

// GetStoredBlock reads a block with its child list: a store that knows its
// refs reports them, and the Volume's liveness records supply the rest, unknown
// for a block stored without one.
func (r storeReader) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read the block and mark it durable.
	gen := r.readGen()
	stored, err := r.StoreOps.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return stored, err
	}
	block.MarkDurableRead(ctx, ref, gen)

	// Fill in its child list.
	if !stored.RefsKnown {
		stored.Refs, stored.RefsKnown, err = r.volume.GetChildren(ctx, ref)
	}
	return stored, err
}

// GetBlockExists reports whether the store holds ref, hiding an unmarked
// block, as Volume.hideUnmarked describes.
func (r storeReader) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Check the store, then hide an unmarked block.
	found, err := r.StoreOps.GetBlockExists(ctx, ref)
	if err != nil || !found {
		return found, err
	}
	exists := []bool{true}
	err = r.volume.hideUnmarked(ctx, []*block.BlockRef{ref}, exists)
	return exists[0], err
}

// GetBlockExistsBatch reports whether the store holds each of refs, hiding
// unmarked blocks, as Volume.hideUnmarked describes.
func (r storeReader) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	exists, err := r.StoreOps.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, err
	}
	return exists, r.volume.hideUnmarked(ctx, refs, exists)
}

// RmBlock removes a block from the store and reports the removal.
func (r storeReader) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	defer block.InvalidateRemovedBlock(ref)
	return r.StoreOps.RmBlock(ctx, ref)
}

// _ is a type assertion
var _ block.StoreOps = storeReader{}
