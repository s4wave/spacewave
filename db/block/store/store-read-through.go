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
// This owner is separate from block.StoreOverlay because StoreOverlay readback
// is intentionally asynchronous.
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
	for _, source := range []StoreSource{s.primary, s.lower} {
		if source == nil {
			continue
		}
		if store := source(); store != nil {
			if hashType := store.GetHashType(); hashType != 0 {
				return hashType
			}
		}
	}
	return 0
}

// GetSupportedFeatures returns the primary source's feature set.
func (s *StoreReadThrough) GetSupportedFeatures() block.StoreFeature {
	if s.primary == nil {
		return 0
	}
	if primary := s.primary(); primary != nil {
		return primary.GetSupportedFeatures()
	}
	return 0
}

// BeginReadOperation opens read scopes on the currently available sources.
func (s *StoreReadThrough) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	primary := block.StoreOps(nil)
	if s.primary != nil {
		primary = s.primary()
	}
	if primary == nil {
		return &StoreReadThrough{primary: s.primary, lower: s.lower, writeback: s.writeback}, func() {}, nil
	}
	primary, releasePrimary, err := primary.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}

	lower := block.StoreOps(nil)
	if s.lower != nil {
		lower = s.lower()
	}
	if lower == nil {
		return &StoreReadThrough{
			primary:   func() block.StoreOps { return primary },
			lower:     s.lower,
			writeback: s.writeback,
		}, releasePrimary, nil
	}
	lower, releaseLower, err := lower.BeginReadOperation(ctx)
	if err != nil {
		releasePrimary()
		return nil, nil, err
	}
	return &StoreReadThrough{
			primary:   func() block.StoreOps { return primary },
			lower:     func() block.StoreOps { return lower },
			writeback: s.writeback,
		}, func() {
			releaseLower()
			releasePrimary()
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
// writeback is enabled, a lower hit is synchronously inserted into primary.
func (s *StoreReadThrough) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	primary := block.StoreOps(nil)
	if s.primary != nil {
		primary = s.primary()
	}
	if primary != nil {
		data, found, err := primary.GetBlock(ctx, ref)
		if err != nil || found {
			return data, found, err
		}
	}
	if s.lower == nil {
		return nil, false, nil
	}
	lower := s.lower()
	if lower == nil {
		return nil, false, nil
	}
	data, found, err := lower.GetBlock(ctx, ref)
	if err != nil || !found {
		return data, found, err
	}
	if !s.writeback || primary == nil {
		return data, true, nil
	}
	if _, _, err := primary.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref.Clone()}); err != nil {
		return nil, false, err
	}
	return data, true, nil
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

var _ block.StoreOps = ((*StoreReadThrough)(nil))
