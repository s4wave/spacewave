package engine

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/workload"
	"github.com/s4wave/spacewave/net/hash"
)

// scopedStore overlays local admitted writes on protected immutable file reads.
type scopedStore struct {
	// owner supplies bounded pending read-through state and lifetime checks.
	owner *BlockStore
	// id identifies the read scope in workload records.
	id uint64
	// raw retains the immutable root's reclamation protection.
	raw *packStore
	// pending preserves values admitted before this operation acquired its root.
	pending map[string]*pendingWrite
}

// GetBlock reads pending content or the protected immutable generation.
func (s *scopedStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.getBlock(ctx, ref)
	logBlock(ctx, workload.OpGetBlock, s.id, ref, foundSize(found, len(data)))
	return data, found, err
}

// GetStoredBlock serves the block without refs because this store keeps
// block bytes without their refs.
func (s *scopedStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, s, ref)
}

// getBlock overlays pending content on the protected immutable generation.
func (s *scopedStore) getBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	entry, err := s.pendingEntry(ctx, ref)
	if err != nil {
		return nil, false, err
	}
	if entry != nil {
		return bytes.Clone(entry.Data), !entry.Tombstone, nil
	}
	return s.raw.GetBlock(ctx, ref)
}

// StatBlock resolves indexed length within the protected operation.
func (s *scopedStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	stat, err := s.statBlock(ctx, ref)
	logBlock(ctx, workload.OpStatBlock, s.id, ref, statSize(stat))
	return stat, err
}

// statBlock overlays pending length on the protected immutable index.
func (s *scopedStore) statBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	entry, err := s.pendingEntry(ctx, ref)
	if err != nil {
		return nil, err
	}
	if entry != nil {
		if entry.Tombstone {
			return nil, nil
		}
		return &block.BlockStat{Ref: ref.Clone(), Size: int64(len(entry.Data))}, nil
	}
	return s.raw.StatBlock(ctx, ref)
}

// GetBlockExists answers without reading payload bytes.
func (s *scopedStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	stat, err := s.statBlock(ctx, ref)
	logBlock(ctx, workload.OpBlockExists, s.id, ref, presence(stat != nil))
	return stat != nil, err
}

// GetBlockExistsBatch reuses this operation's protected root for every lookup.
func (s *scopedStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	workload.Record{Op: workload.OpExistsBatch, ID: s.id, Size: int64(len(refs))}.Log(ctx)
	out := make([]bool, len(refs))
	for i, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		var err error
		out[i], err = s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// pendingEntry overlays current admission and the operation's original pending data.
func (s *scopedStore) pendingEntry(ctx context.Context, ref *block.BlockRef) (*block.PutBatchEntry, error) {
	entry, err := s.owner.pendingEntry(ctx, ref)
	if err != nil || entry != nil {
		return entry, err
	}
	key, err := blockKey(ref)
	if err != nil {
		return nil, err
	}
	if pending := s.pending[string(key)]; pending != nil {
		return pending.entry, nil
	}
	return nil, nil
}

// BeginReadOperation reuses this scope without queuing behind its own lease.
func (s *scopedStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	if err := s.owner.check(ctx); err != nil {
		return nil, nil, err
	}
	return s, func() {}, nil
}

// GetHashType returns the owner's content hash selection.
func (s *scopedStore) GetHashType() hash.HashType { return s.owner.GetHashType() }

// GetSupportedFeatures forwards the owner's native storage features.
func (s *scopedStore) GetSupportedFeatures() block.StoreFeature {
	return s.owner.GetSupportedFeatures()
}

// PutBlock admits writes through the owning store.
func (s *scopedStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return s.owner.PutBlock(ctx, data, opts)
}

// PutBlockBatch admits batches through the owning store.
func (s *scopedStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return s.owner.PutBlockBatch(ctx, entries)
}

// RmBlock orders deletion through the owning store.
func (s *scopedStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return s.owner.RmBlock(ctx, ref)
}

// Sync fences admission through the owning store.
func (s *scopedStore) Sync(ctx context.Context) (bool, error) { return s.owner.Sync(ctx) }

// _ verifies explicit forwarding of the complete storage interface.
var _ block.StoreOps = (*scopedStore)(nil)
