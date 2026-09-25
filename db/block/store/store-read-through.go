package block_store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// StoreSource resolves the current store for a read pipeline.
//
// A nil result means that the source is unavailable and the pipeline moves to
// its next source.
type StoreSource func() block.StoreOps

// StoreReadThrough reads from a primary source, then an optional lower source.
// When writeback is enabled, lower hits are synchronously written to primary.
//
// StoreReadThrough is separate from block.StoreOverlay because StoreOverlay
// readback is asynchronous.
type StoreReadThrough struct {
	primary   StoreSource
	lower     StoreSource
	writeback bool
}

// NewStoreReadThrough constructs a synchronous read-through store.
func NewStoreReadThrough(primary, lower StoreSource, writeback bool) *StoreReadThrough {
	return &StoreReadThrough{primary: primary, lower: lower, writeback: writeback}
}

// GetHashType returns the first available source hash type.
func (s *StoreReadThrough) GetHashType() hash.HashType {
	for _, src := range []StoreSource{s.primary, s.lower} {
		if store := s.source(src); store != nil {
			if hashType := store.GetHashType(); hashType != 0 {
				return hashType
			}
		}
	}
	return 0
}

// GetSupportedFeatures returns the primary source's feature set.
func (s *StoreReadThrough) GetSupportedFeatures() block.StoreFeature {
	if primary := s.source(s.primary); primary != nil {
		return primary.GetSupportedFeatures()
	}
	return 0
}

// BeginReadOperation opens read scopes on the currently available sources.
// Writeback keeps the primary source writable because lower-source hits are
// inserted into it before the scoped read returns.
func (s *StoreReadThrough) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	primary := s.source(s.primary)
	lower := s.source(s.lower)

	var releasePrimary, releaseLower func()
	if primary != nil && !s.writeback {
		scoped, release, err := primary.BeginReadOperation(ctx)
		if err != nil {
			return nil, nil, err
		}
		primary = scoped
		releasePrimary = release
	}
	if lower != nil {
		scoped, release, err := lower.BeginReadOperation(ctx)
		if err != nil {
			if releasePrimary != nil {
				releasePrimary()
			}
			return nil, nil, err
		}
		lower = scoped
		releaseLower = release
	}

	scoped := &StoreReadThrough{
		primary:   s.primary,
		lower:     s.lower,
		writeback: s.writeback,
	}
	if primary != nil {
		scoped.primary = func() block.StoreOps { return primary }
	}
	if lower != nil {
		scoped.lower = func() block.StoreOps { return lower }
	}
	return scoped, func() {
		if releaseLower != nil {
			releaseLower()
		}
		if releasePrimary != nil {
			releasePrimary()
		}
	}, nil
}

// PutBlock is unsupported because this store is read-only.
func (*StoreReadThrough) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, ErrReadOnly
}

// PutBlockBatch is unsupported because this store is read-only.
func (*StoreReadThrough) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return ErrReadOnly
}

// RmBlock is unsupported because this store is read-only.
func (*StoreReadThrough) RmBlock(context.Context, *block.BlockRef) error {
	return ErrReadOnly
}

// Sync reports that no durability barrier is needed for this read-only view.
func (*StoreReadThrough) Sync(context.Context) (bool, error) { return true, nil }

// GetBlock reads the primary source, then the current lower source. When
// writeback is enabled, a lower hit with known refs is synchronously inserted
// into primary with its refs. A lower hit with unknown refs is served without
// the writeback.
func (s *StoreReadThrough) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	primary := s.source(s.primary)
	if primary != nil {
		data, found, err := primary.GetBlock(ctx, ref)
		if err != nil || found {
			return data, found, err
		}
	}
	lower := s.source(s.lower)
	if lower == nil {
		return nil, false, nil
	}
	if !s.writeback || primary == nil {
		return lower.GetBlock(ctx, ref)
	}
	stored, err := s.readLower(ctx, primary, lower, ref)
	if err != nil || stored == nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

// GetStoredBlock reads the block and its refs from the primary source, then
// the current lower source. The refs come from the source that answered. When
// writeback is enabled, a lower hit with known refs is synchronously inserted
// into primary with its refs.
func (s *StoreReadThrough) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	primary := s.source(s.primary)
	if primary != nil {
		stored, err := primary.GetStoredBlock(ctx, ref)
		if err != nil || stored != nil {
			return stored, err
		}
	}
	lower := s.source(s.lower)
	if lower == nil {
		return nil, nil
	}
	return s.readLower(ctx, primary, lower, ref)
}

// source resolves an optional store source.
func (s *StoreReadThrough) source(src StoreSource) block.StoreOps {
	if src == nil {
		return nil
	}
	return src()
}

// readLower reads a block and its refs from lower. With writeback enabled, a
// hit with known refs is written into primary with them; a block with unknown
// refs would look like a leaf there, so it is served without the writeback.
func (s *StoreReadThrough) readLower(ctx context.Context, primary, lower block.StoreOps, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := lower.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, err
	}
	if s.writeback && primary != nil && stored.RefsKnown {
		if _, _, err := primary.PutBlock(ctx, stored.Data, stored.PutOpts(ref)); err != nil {
			return nil, err
		}
	}
	return stored, nil
}

// GetBlockExists follows the same source order as GetBlock.
func (s *StoreReadThrough) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

// GetBlockExistsBatch follows the same source order as GetBlock.
func (s *StoreReadThrough) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// StatBlock follows the same source order as GetBlock.
func (s *StoreReadThrough) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

// _ is a type assertion
var _ block.StoreOps = (*StoreReadThrough)(nil)
