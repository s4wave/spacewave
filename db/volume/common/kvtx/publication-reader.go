package kvtx

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// publicationReader reads a Volume's blocks with its admitted publications
// laid over the store. An entry of a queued or unapplied publication answers
// before the store, so a reader sees admitted blocks before their group
// applies; the entry leaves the read set when its group resolves, applied or
// failed, and a request known to fail leaves it at once. The writer verifies
// entries at admission, so reads return them without hashing.
//
// For the decoded-block cache, a block the store serves is durable: the store
// is the Volume's committed store, and every removal from it reports itself
// (RmBlock, tombstones, and physicalTx). An admitted entry is staged until
// its group commits, and its group can still fail.
type publicationReader struct {
	// writer holds the admitted publications, nil without atomic publication.
	writer *publicationWriter
	// store reads and writes the applied blocks.
	store block.StoreOps
	// pinned holds the requests admitted when a read scope opened, and is nil
	// outside a read scope, which reads the live queue.
	pinned []*publicationRequest
	// scoped is set in a read scope, which reads one store snapshot.
	scoped bool
	// gen is the removal generation recorded before a read scope's snapshot
	// opened. Outside a read scope each read records its own.
	gen uint64
}

// GetHashType returns the store's preferred hash type.
func (r publicationReader) GetHashType() hash.HashType {
	return r.store.GetHashType()
}

// GetSupportedFeatures returns the store's native features.
func (r publicationReader) GetSupportedFeatures() block.StoreFeature {
	return r.store.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope on the store that still reads
// admitted entries first. The scope serves the requests admitted before its
// store snapshot opens: one that applies later is absent from the snapshot, so
// the scope keeps serving it.
func (r publicationReader) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// A read scope reads its pinned requests and its snapshot for its life.
	if r.scoped {
		return r, func() {}, nil
	}

	// Pin the admitted requests before the snapshot opens, so a group that
	// applies in between is both pinned and in the snapshot. Record the
	// removal generation before the snapshot opens too.
	pinned := r.writer.pin()
	gen := block.DurableReadGeneration()
	scope, release, err := r.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return publicationReader{writer: r.writer, store: scope, pinned: pinned, scoped: true, gen: gen}, release, nil
}

// PutBlock writes a block to the store.
func (r publicationReader) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return r.store.PutBlock(ctx, data, opts)
}

// PutBlockBatch writes a batch of blocks to the store, reporting its
// tombstones as removals.
func (r publicationReader) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	defer block.InvalidateRemovedTombstones(entries)
	return r.store.PutBlockBatch(ctx, entries)
}

// readGen returns the removal generation for a read about to open: the read
// scope's, or the current one outside a scope.
func (r publicationReader) readGen() uint64 {
	if r.scoped {
		return r.gen
	}
	return block.DurableReadGeneration()
}

// GetBlock reads an admitted entry before the store.
func (r publicationReader) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Read the store when no admitted publication writes the block, and mark
	// a found block durable.
	gen := r.readGen()
	entry, err := r.writer.find(ref, r.pinned)
	if err != nil {
		return nil, false, err
	}
	if entry == nil {
		data, found, err := r.store.GetBlock(ctx, ref)
		if err == nil && found {
			block.MarkDurableRead(ctx, ref, gen)
		}
		return data, found, err
	}

	// Serve the admitted bytes, or absence for a tombstone. The bytes are
	// staged until the entry's group commits.
	if entry.Tombstone {
		return nil, false, nil
	}
	block.MarkStagedRead(ctx)
	return bytes.Clone(entry.Data), true, nil
}

// GetStoredBlock reads an admitted entry before the store. An admitted entry
// carries no refs, as the store keeps none for the block once it applies.
func (r publicationReader) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read the store when no admitted publication writes the block, so a store
	// that knows its refs reports them, and mark a found block durable.
	gen := r.readGen()
	entry, err := r.writer.find(ref, r.pinned)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		stored, err := r.store.GetStoredBlock(ctx, ref)
		if err == nil && stored != nil {
			block.MarkDurableRead(ctx, ref, gen)
		}
		return stored, err
	}

	// Serve the admitted bytes, or absence for a tombstone. The bytes are
	// staged until the entry's group commits.
	if entry.Tombstone {
		return nil, nil
	}
	block.MarkStagedRead(ctx)
	return &block.StoredBlock{Data: bytes.Clone(entry.Data)}, nil
}

// GetBlockExists reports an admitted entry before the store.
func (r publicationReader) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	entry, err := r.writer.find(ref, r.pinned)
	if err != nil {
		return false, err
	}
	if entry == nil {
		return r.store.GetBlockExists(ctx, ref)
	}
	return !entry.Tombstone, nil
}

// GetBlockExistsBatch reports admitted entries and probes the store for the
// rest in one batch.
func (r publicationReader) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Answer each reference an admitted entry writes.
	out := make([]bool, len(refs))
	var missing []*block.BlockRef
	var missingIdx []int
	for i, ref := range refs {
		entry, err := r.writer.find(ref, r.pinned)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			out[i] = !entry.Tombstone
			continue
		}
		missing = append(missing, ref)
		missingIdx = append(missingIdx, i)
	}

	// Probe the store for the remaining references.
	if len(missing) == 0 {
		return out, nil
	}
	found, err := r.store.GetBlockExistsBatch(ctx, missing)
	if err != nil {
		return nil, err
	}
	for i, idx := range missingIdx {
		out[idx] = found[i]
	}
	return out, nil
}

// RmBlock removes a block from the store and reports the removal.
func (r publicationReader) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	defer block.InvalidateRemovedBlock(ref)
	return r.store.RmBlock(ctx, ref)
}

// StatBlock reports an admitted entry's size before the store.
func (r publicationReader) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Stat the store when no admitted publication writes the block.
	entry, err := r.writer.find(ref, r.pinned)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return r.store.StatBlock(ctx, ref)
	}

	// Report the admitted size, or absence for a tombstone.
	if entry.Tombstone {
		return nil, nil
	}
	return &block.BlockStat{Ref: ref.Clone(), Size: int64(len(entry.Data))}, nil
}

// Sync syncs the store.
func (r publicationReader) Sync(ctx context.Context) (bool, error) {
	return r.store.Sync(ctx)
}

// _ is a type assertion
var _ block.StoreOps = publicationReader{}
