package block_store

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// StoreSource resolves the current store for a read pipeline.
//
// A nil result means that the source is unavailable and the pipeline moves to
// its next source.
type StoreSource func() block.StoreOps

// Fill writes a block read from the lower source, with its known refs, into
// the primary source.
type Fill func(ctx context.Context, primary block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error

// CacheFill writes the block into primary as a cache fill, which takes no
// ownership.
func CacheFill(ctx context.Context, primary block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error {
	opts := stored.PutOpts(ref)
	opts.CacheFill = true
	_, _, err := primary.PutBlock(ctx, stored.Data, opts)
	return err
}

// StoreReadThrough reads from a primary source, then an optional lower source.
// With a fill, lower hits are written to primary in the background.
//
// StoreReadThrough is separate from block.StoreOverlay because StoreOverlay
// readback is asynchronous.
type StoreReadThrough struct {
	primary StoreSource
	lower   StoreSource
	fill    Fill
}

// NewStoreReadThrough constructs a synchronous read-through store. A nil fill
// serves lower hits without writing them back.
func NewStoreReadThrough(primary, lower StoreSource, fill Fill) *StoreReadThrough {
	return &StoreReadThrough{primary: primary, lower: lower, fill: fill}
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
// A fill keeps the primary source writable because lower-source hits are
// inserted into it before the scoped read returns.
func (s *StoreReadThrough) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// Resolve the primary and lower stores for this read operation.
	primary := s.source(s.primary)
	lower := s.source(s.lower)

	// Scope the primary store when lower hits will not be written back.
	var releasePrimary, releaseLower func()
	if primary != nil && s.fill == nil {
		scoped, release, err := primary.BeginReadOperation(ctx)
		if err != nil {
			return nil, nil, err
		}
		primary = scoped
		releasePrimary = release
	}

	// Scope the lower store and release the primary scope if acquisition fails.
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

	// Bind the read-through view to the acquired scopes and their cleanup.
	scoped := &StoreReadThrough{
		primary: s.primary,
		lower:   s.lower,
		fill:    s.fill,
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

// GetRoot reads the root from the primary source.
func (s *StoreReadThrough) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	primary := s.source(s.primary)
	if primary == nil {
		return nil, nil
	}
	return primary.GetRoot(ctx, name)
}

// ListRoots lists the roots of the primary source.
func (s *StoreReadThrough) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	primary := s.source(s.primary)
	if primary == nil {
		return nil
	}
	return primary.ListRoots(ctx, prefix, fn)
}

// SetRoots is unsupported because this store is read-only.
func (*StoreReadThrough) SetRoots(context.Context, []*block.PutBatchEntry, []block.NamedRoot) error {
	return ErrReadOnly
}

// PutBlock is unsupported because this store is read-only.
func (*StoreReadThrough) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, ErrReadOnly
}

// PutBlockBatch is unsupported because this store is read-only.
func (*StoreReadThrough) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, ErrReadOnly
}

// RmBlock is unsupported because this store is read-only.
func (*StoreReadThrough) RmBlock(context.Context, *block.BlockRef) error {
	return ErrReadOnly
}

// Sync reports that no durability barrier is needed for this read-only view.
func (*StoreReadThrough) Sync(context.Context) (bool, error) { return true, nil }

// GetBlock reads the primary source, then the current lower source. With a
// fill, a lower hit with known refs is inserted into primary in the
// background with its refs. A lower hit with unknown refs is served without the fill.
func (s *StoreReadThrough) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Serve the block from the primary store when it is available there.
	primary := s.source(s.primary)
	if primary != nil {
		data, found, err := primary.GetBlock(ctx, ref)
		if err != nil || found {
			return data, found, err
		}
	}

	// Resolve the lower store and serve directly without a fill.
	lower := s.source(s.lower)
	if lower == nil {
		return nil, false, s.lowerUnavailable(ctx, ref)
	}
	if s.fill == nil || primary == nil {
		return lower.GetBlock(ctx, ref)
	}

	// Read the lower block and fill the primary store when its refs are known.
	stored, err := s.readLower(ctx, primary, lower, ref)
	if err != nil || stored == nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

// GetStoredBlock reads the block and its refs from the primary source, then
// the current lower source. The refs come from the source that answered. With
// a fill, a lower hit with known refs is inserted into primary in the
// background with its refs.
func (s *StoreReadThrough) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Serve the stored block and its refs from the primary store when present.
	primary := s.source(s.primary)
	if primary != nil {
		stored, err := primary.GetStoredBlock(ctx, ref)
		if err != nil || stored != nil {
			return stored, err
		}
	}

	// Resolve the lower store for the stored block lookup.
	lower := s.source(s.lower)
	if lower == nil {
		return nil, s.lowerUnavailable(ctx, ref)
	}
	return s.readLower(ctx, primary, lower, ref)
}

// lowerUnavailable returns the outcome of a primary miss while the lower
// source resolves to nil. A read that may not wait for a peer records the
// block and reports it unavailable, since the source may hold it once it is
// back; any other read, or a pipeline without a lower source, reports a miss.
func (s *StoreReadThrough) lowerUnavailable(ctx context.Context, ref *block.BlockRef) error {
	peerWait := block.GetPeerWait(ctx)
	if s.lower == nil || peerWait == nil {
		return nil
	}
	peerWait.Skip(ref)
	return errors.Wrapf(block.ErrUnavailable, "lower store unavailable for block %s", ref.MarshalString())
}

// source resolves an optional store source.
func (s *StoreReadThrough) source(src StoreSource) block.StoreOps {
	if src == nil {
		return nil
	}
	return src()
}

// readLower reads a block and its refs from lower. With a fill, a hit with
// known refs is written into primary before the read returns; a block with
// unknown refs would look like a leaf there, so it is served without the fill.
// A fill that fails leaves the block readable from lower, so its error is
// dropped and the next read fills it again. A placed store's fill only queues
// the block for its store's background drain.
func (s *StoreReadThrough) readLower(ctx context.Context, primary, lower block.StoreOps, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := lower.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, err
	}
	if s.fill != nil && primary != nil && stored.RefsKnown {
		_ = s.fill(context.WithoutCancel(ctx), primary, ref, stored)
	}
	return stored, nil
}

// GetBlockExists checks the primary source, then the current lower source.
// It asks each source for existence rather than reading the block, so a lower
// source can answer without waiting to serve data, and nothing is written back.
func (s *StoreReadThrough) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Report a block the primary store holds.
	primary := s.source(s.primary)
	if primary != nil {
		found, err := primary.GetBlockExists(ctx, ref)
		if err != nil || found {
			return found, err
		}
	}

	// Ask the lower store about a primary miss.
	lower := s.source(s.lower)
	if lower == nil {
		return false, s.lowerUnknown(ctx)
	}
	return lower.GetBlockExists(ctx, ref)
}

// lowerUnknown returns the outcome of an existence miss while the lower source
// resolves to nil. A configured lower source that is not placed yet may hold
// the block once it is, so a caller that may not wait for a peer, which treats
// a false answer as proof the block is gone, learns the answer is unknown. Any
// other caller, or a pipeline without a lower source, reads a miss. A source
// that is known to hold nothing, such as a Space without peers, resolves to a
// nil StoreSource rather than a source returning nil.
func (s *StoreReadThrough) lowerUnknown(ctx context.Context) error {
	if s.lower == nil || block.GetPeerWait(ctx) == nil {
		return nil
	}
	return errors.Wrap(block.ErrUnavailable, "lower store unavailable")
}

// GetBlockExistsBatch checks the primary source for every ref, then asks the
// current lower source about the primary misses.
func (s *StoreReadThrough) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Check the primary store for every ref.
	out := make([]bool, len(refs))
	primary := s.source(s.primary)
	if primary != nil {
		found, err := primary.GetBlockExistsBatch(ctx, refs)
		if err != nil {
			return nil, err
		}
		copy(out, found)
	}

	// Collect the primary misses.
	lower := s.source(s.lower)
	if lower == nil {
		if slices.Contains(out, false) {
			if err := s.lowerUnknown(ctx); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	var missIdx []int
	var missRefs []*block.BlockRef
	for i, found := range out {
		if !found {
			missIdx = append(missIdx, i)
			missRefs = append(missRefs, refs[i])
		}
	}
	if len(missRefs) == 0 {
		return out, nil
	}

	// Ask the lower store about the misses.
	found, err := lower.GetBlockExistsBatch(ctx, missRefs)
	if err != nil {
		return nil, err
	}
	for j, i := range missIdx {
		out[i] = found[j]
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
