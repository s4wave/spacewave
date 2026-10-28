package block_store

import (
	"context"
	"errors"
	"fmt"
	"slices"

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
// A read combines the sources' answers: a hit from either source serves it, a
// source that failed or could not answer does not keep the read from the next
// source, and a read neither source served reports the failures, or a miss when
// each source said it lacks the block. A read whose context ended stops at once.
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
	// Serve the block from the primary store when it is there.
	var misses readMisses
	primary := s.source(s.primary)
	if primary != nil {
		data, found, err := primary.GetBlock(ctx, ref)
		if found {
			return data, true, err
		}
		if !misses.add(ctx, err) {
			return nil, false, err
		}
	}

	// Read the lower store, with its refs when a fill can take the block.
	lower := s.source(s.lower)
	var err error
	switch {
	case lower == nil:
		err = s.lowerUnavailable(ctx, ref)
	case s.fill == nil || primary == nil:
		var data []byte
		var found bool
		data, found, err = lower.GetBlock(ctx, ref)
		if found {
			return data, true, err
		}
	default:
		var stored *block.StoredBlock
		stored, err = s.readLower(ctx, primary, lower, ref)
		if stored != nil {
			return stored.Data, true, nil
		}
	}

	// Report what kept both sources from serving the block.
	if !misses.add(ctx, err) {
		return nil, false, err
	}
	return nil, false, misses.err()
}

// GetStoredBlock reads the block and its refs from the primary source, then
// the current lower source. The refs come from the source that answered. With
// a fill, a lower hit with known refs is inserted into primary in the
// background with its refs.
func (s *StoreReadThrough) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Serve the stored block and its refs from the primary store when present.
	var misses readMisses
	primary := s.source(s.primary)
	if primary != nil {
		stored, err := primary.GetStoredBlock(ctx, ref)
		if stored != nil {
			return stored, err
		}
		if !misses.add(ctx, err) {
			return nil, err
		}
	}

	// Read the stored block from the lower store.
	lower := s.source(s.lower)
	var err error
	if lower == nil {
		err = s.lowerUnavailable(ctx, ref)
	} else {
		var stored *block.StoredBlock
		stored, err = s.readLower(ctx, primary, lower, ref)
		if stored != nil {
			return stored, nil
		}
	}

	// Report what kept both sources from serving the block.
	if !misses.add(ctx, err) {
		return nil, err
	}
	return nil, misses.err()
}

// readMisses collects the answers of the sources that did not serve a read.
type readMisses struct {
	// miss is the last miss a source reported as an error.
	miss error
	// unavailable holds the answers of sources that could not answer yet.
	unavailable []error
	// failed holds the failures of sources that could not answer at all.
	failed []error
}

// add records a source's answer to a read it did not serve. It reports false
// when the read's context ended, and the read stops with err.
func (m *readMisses) add(ctx context.Context, err error) bool {
	switch {
	case err == nil:
	case ctx.Err() != nil:
		return false
	case errors.Is(err, block.ErrUnavailable):
		m.unavailable = append(m.unavailable, err)
	case errors.Is(err, block.ErrNotFound):
		m.miss = err
	default:
		m.failed = append(m.failed, err)
	}
	return true
}

// err returns the outcome of a read no source served: a failure first, so a
// caller never retries a broken source as a block not here yet, then the
// unavailable answers, then the miss, which is nil unless a source reported
// it as an error.
func (m *readMisses) err() error {
	switch {
	case len(m.failed) != 0:
		return joinReadErrs(m.failed)
	case len(m.unavailable) != 0:
		return joinReadErrs(m.unavailable)
	}
	return m.miss
}

// joinReadErrs returns the one error of errs as is, or joins several.
func joinReadErrs(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
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
	return fmt.Errorf("%w: lower store unavailable for block %s", block.ErrUnavailable, ref.MarshalString())
}

// source resolves an optional store source.
func (s *StoreReadThrough) source(src StoreSource) block.StoreOps {
	if src == nil {
		return nil
	}
	return src()
}

// readLower reads a block and its refs from lower. With a fill, a hit with
// known refs is written into primary; a block with unknown refs would look
// like a leaf there, so it is served without the fill.
func (s *StoreReadThrough) readLower(ctx context.Context, primary, lower block.StoreOps, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := lower.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, err
	}
	if s.fill != nil && primary != nil && stored.RefsKnown {
		// Fill in the background so the read does not wait on the primary's
		// writer, which a standby's backfill can hold for seconds. A fill that
		// finds the pool full is skipped; the next read fills it again.
		select {
		case fillSlots <- struct{}{}:
			fillCtx := context.WithoutCancel(ctx)
			go func() {
				defer func() { <-fillSlots }()
				_ = s.fill(fillCtx, primary, ref, stored)
			}()
		default:
		}
	}
	return stored, nil
}

// fillSlots bounds the background fills of every read-through store.
var fillSlots = make(chan struct{}, 256)

// GetBlockExists checks the primary source, then the current lower source.
// It asks each source for existence rather than reading the block, so a lower
// source can answer without waiting to serve data, and nothing is written back.
func (s *StoreReadThrough) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Report a block the primary store holds.
	var misses readMisses
	primary := s.source(s.primary)
	if primary != nil {
		found, err := primary.GetBlockExists(ctx, ref)
		if found {
			return true, err
		}
		if !misses.add(ctx, err) {
			return false, err
		}
	}

	// Ask the lower store about a block the primary store did not report.
	lower := s.source(s.lower)
	if lower == nil {
		return false, misses.err()
	}
	found, err := lower.GetBlockExists(ctx, ref)
	if found {
		return true, err
	}
	if !misses.add(ctx, err) {
		return false, err
	}
	return false, misses.err()
}

// GetBlockExistsBatch checks the primary source for every ref, then asks the
// current lower source about the refs the primary source did not report. It
// fails only when a source failed and a ref stayed unreported.
func (s *StoreReadThrough) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Check the primary store for every ref.
	var misses readMisses
	out := make([]bool, len(refs))
	primary := s.source(s.primary)
	if primary != nil {
		found, err := primary.GetBlockExistsBatch(ctx, refs)
		if !misses.add(ctx, err) {
			return nil, err
		}
		if err == nil {
			copy(out, found)
		}
	}

	// Collect the refs the primary store did not report.
	lower := s.source(s.lower)
	var missIdx []int
	var missRefs []*block.BlockRef
	for i, found := range out {
		if !found {
			missIdx = append(missIdx, i)
			missRefs = append(missRefs, refs[i])
		}
	}

	// Ask the lower store about them.
	if lower != nil && len(missRefs) != 0 {
		found, err := lower.GetBlockExistsBatch(ctx, missRefs)
		if !misses.add(ctx, err) {
			return nil, err
		}
		for j, i := range missIdx {
			out[i] = err == nil && found[j]
		}
	}

	// Fail when a source failed and left a ref unreported.
	if err := misses.err(); err != nil && slices.Contains(out, false) {
		return nil, err
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
