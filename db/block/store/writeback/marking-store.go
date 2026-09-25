package block_store_writeback

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// Mark names a written block.
type Mark struct {
	// Hash is the block hash.
	Hash *hash.Hash
	// Size is the block size in bytes.
	Size int64
}

// MarkFunc durably records written blocks before their writes are
// acknowledged. A batch write passes all of its blocks in one call.
type MarkFunc func(ctx context.Context, marks []Mark) error

// decodedBlockRefInvalidator removes decoded values after storage mutation.
type decodedBlockRefInvalidator interface {
	InvalidateDecodedBlockRef(context.Context, *block.BlockRef)
}

// MarkingStore acknowledges writes only after mark retains each written block.
//
// A write that fails to mark returns the error with the block stored;
// repeating the write repairs the marker.
type MarkingStore struct {
	// store owns the block writes.
	store block.StoreOps
	// mark records every written block.
	mark MarkFunc
}

// NewMarkingStore wraps store, calling mark after each successful write.
func NewMarkingStore(store block.StoreOps, mark MarkFunc) *MarkingStore {
	return &MarkingStore{store: store, mark: mark}
}

// GetHashType returns the inner store hash type.
func (m *MarkingStore) GetHashType() hash.HashType {
	return m.store.GetHashType()
}

// GetSupportedFeatures returns the inner store native feature bitset.
func (m *MarkingStore) GetSupportedFeatures() block.StoreFeature {
	return m.store.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope on the inner store.
func (m *MarkingStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := m.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &MarkingStore{store: store, mark: m.mark}, release, nil
}

// PutBlock stores a block and marks it, even when it already existed.
func (m *MarkingStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := m.store.PutBlock(ctx, data, opts)
	if err == nil && !ref.GetEmpty() {
		err = m.mark(ctx, []Mark{{Hash: ref.GetHash(), Size: int64(len(data))}})
	}
	return ref, existed, err
}

// PutBlockBatch marks every successful non-tombstone write in one call.
// A failed marker returns an error; repeating the batch repairs the markers.
func (m *MarkingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if err := m.store.PutBlockBatch(ctx, entries); err != nil {
		return err
	}

	marks := make([]Mark, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tombstone {
			m.invalidate(ctx, entry.Ref)
			continue
		}
		if entry.Ref.GetEmpty() {
			continue
		}
		marks = append(marks, Mark{Hash: entry.Ref.GetHash(), Size: int64(len(entry.Data))})
	}
	if len(marks) == 0 {
		return nil
	}
	return m.mark(ctx, marks)
}

// GetBlock gets a block by reference.
func (m *MarkingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return m.store.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists.
func (m *MarkingStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return m.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards batched existence probes to the inner store.
func (m *MarkingStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return m.store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock removes a block.
func (m *MarkingStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := m.store.RmBlock(ctx, ref); err != nil {
		return err
	}
	m.invalidate(ctx, ref)
	return nil
}

// StatBlock returns block metadata.
func (m *MarkingStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return m.store.StatBlock(ctx, ref)
}

// Sync forwards the durability barrier to the inner store.
func (m *MarkingStore) Sync(ctx context.Context) (bool, error) {
	return m.store.Sync(ctx)
}

// BeginDeferFlush forwards the GC ref-batch scope to the inner store.
func (m *MarkingStore) BeginDeferFlush() {
	block.BeginDeferFlush(m.store)
}

// EndDeferFlush forwards the GC ref-batch scope to the inner store.
func (m *MarkingStore) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, m.store)
}

// invalidate drops decoded values for a removed block when the store caches them.
func (m *MarkingStore) invalidate(ctx context.Context, ref *block.BlockRef) {
	if invalidator, ok := m.store.(decodedBlockRefInvalidator); ok {
		invalidator.InvalidateDecodedBlockRef(ctx, ref)
	}
}

// _ is a type assertion
var _ block.StoreOps = (*MarkingStore)(nil)
