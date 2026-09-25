package block

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// StoreOverlay layers an upper block store over a lower store.
//
// ctx is used for writeback requests
type StoreOverlay struct {
	// ctx owns writeback request lifetimes.
	ctx context.Context
	// le is the logger.
	le *logrus.Entry
	// lower is the backing store; upper is the cache/front store.
	lower, upper StoreOps
	// mode selects the read and writeback routing.
	mode OverlayMode
	// writebackTimeout bounds each writeback, zero means no timeout.
	writebackTimeout time.Duration
	// writebackPutOpts carries options for writeback puts.
	writebackPutOpts *PutOpts
}

// NewOverlay constructs a new overlay store.
//
// ctx is used for writeback requests
func NewOverlay(
	ctx context.Context,
	le *logrus.Entry,
	lower,
	upper StoreOps,
	mode OverlayMode,
	writebackTimeout time.Duration,
	writebackPutOpts *PutOpts,
) *StoreOverlay {
	return &StoreOverlay{
		ctx:   ctx,
		le:    le,
		lower: lower,
		upper: upper,
		mode:  mode,

		writebackTimeout: writebackTimeout,
		writebackPutOpts: writebackPutOpts,
	}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (o *StoreOverlay) GetHashType() hash.HashType {
	if v := o.upper.GetHashType(); v != 0 {
		return v
	}
	return o.lower.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask for the overlay.
func (o *StoreOverlay) GetSupportedFeatures() StoreFeature {
	upper := o.upper.GetSupportedFeatures()
	lower := o.lower.GetSupportedFeatures()
	both := upper & lower
	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		return upper
	case OverlayMode_LOWER_ONLY:
		return lower
	case OverlayMode_UPPER_CACHE, OverlayMode_LOWER_CACHE:
		return both
	case OverlayMode_UPPER_READ_CACHE:
		return (lower & StoreFeatureNativeBatchPut) |
			(both & StoreFeatureNativeBatchExists)
	case OverlayMode_LOWER_READ_CACHE:
		return (upper & StoreFeatureNativeBatchPut) |
			(both & StoreFeatureNativeBatchExists)
	case OverlayMode_UPPER_WRITE_CACHE, OverlayMode_UPPER_READBACK_CACHE:
		return (upper & StoreFeatureNativeBatchPut) |
			(both & StoreFeatureNativeBatchExists)
	case OverlayMode_LOWER_WRITE_CACHE:
		return (lower & StoreFeatureNativeBatchPut) |
			(both & StoreFeatureNativeBatchExists)
	}
}

// BeginReadOperation opens read scopes for the overlay stores.
func (o *StoreOverlay) BeginReadOperation(ctx context.Context) (StoreOps, func(), error) {
	lower, lowerRelease, err := o.lower.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	upper, upperRelease, err := o.upper.BeginReadOperation(ctx)
	if err != nil {
		lowerRelease()
		return nil, nil, err
	}
	scoped := *o
	scoped.lower = lower
	scoped.upper = upper
	return &scoped, func() {
		upperRelease()
		lowerRelease()
	}, nil
}

// readRoute returns the stores a read tries in order and the store a hit in
// the second store fills. second is nil when reads use one store, and fill is
// nil when reads do not fill.
func (o *StoreOverlay) readRoute() (first, second, fill StoreOps) {
	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		return o.upper, nil, nil
	case OverlayMode_LOWER_ONLY:
		return o.lower, nil, nil
	case OverlayMode_UPPER_CACHE, OverlayMode_UPPER_READBACK_CACHE:
		return o.upper, o.lower, o.upper
	case OverlayMode_LOWER_CACHE:
		return o.lower, o.upper, o.lower
	case OverlayMode_UPPER_READ_CACHE, OverlayMode_UPPER_WRITE_CACHE:
		return o.upper, o.lower, nil
	case OverlayMode_LOWER_READ_CACHE, OverlayMode_LOWER_WRITE_CACHE:
		return o.lower, o.upper, nil
	}
}

// GetBlock gets a block with the given reference.
// The ref should not be modified or retained by GetBlock.
// Returns data, found, error.
// Returns nil, false, nil if not found.
// Note: the block may not be in the specified bucket.
func (o *StoreOverlay) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	first, second, fill := o.readRoute()
	data, found, err := first.GetBlock(ctx, ref)
	if err != nil || found || second == nil {
		return data, found, err
	}
	if fill == nil {
		return second.GetBlock(ctx, ref)
	}
	stored, err := o.readFill(ctx, second, fill, ref)
	if err != nil || stored == nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

// GetStoredBlock gets a block and its references using the read policy of
// GetBlock. Fills carry the references read from the answering store.
func (o *StoreOverlay) GetStoredBlock(ctx context.Context, ref *BlockRef) (*StoredBlock, error) {
	first, second, fill := o.readRoute()
	stored, err := first.GetStoredBlock(ctx, ref)
	if err != nil || stored != nil || second == nil {
		return stored, err
	}
	return o.readFill(ctx, second, fill, ref)
}

// readFill reads a block from the second store and fills it into fill when
// set. A fill carries the block's edges; without them the block would look
// like a leaf, so a block with unknown refs is served without filling.
func (o *StoreOverlay) readFill(ctx context.Context, second, fill StoreOps, ref *BlockRef) (*StoredBlock, error) {
	stored, err := second.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, err
	}
	if fill != nil && stored.RefsKnown {
		o.fill(fill, ref, stored)
	}
	return stored, nil
}

// fill writes a block read from the second store back to the first store in
// the background, bounded by the overlay context and writeback timeout.
func (o *StoreOverlay) fill(target StoreOps, ref *BlockRef, stored *StoredBlock) {
	if o.ctx.Err() != nil {
		return
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if o.writebackTimeout > 0 {
		ctx, cancel = context.WithTimeout(o.ctx, o.writebackTimeout)
	} else {
		ctx, cancel = context.WithCancel(o.ctx)
	}
	putOpts := o.writebackPutOpts.CloneVT()
	if putOpts == nil {
		putOpts = &PutOpts{}
	}
	putOpts.ForceBlockRef = ref.Clone()
	putOpts.Refs = CloneBlockRefs(stored.Refs)
	go func() {
		defer cancel()
		if _, _, err := target.PutBlock(ctx, stored.Data, putOpts); err != nil {
			o.le.WithError(err).Debug("block overlay writeback failed")
		}
	}()
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (o *StoreOverlay) GetBlockExists(ctx context.Context, ref *BlockRef) (bool, error) {
	first, second, _ := o.readRoute()
	found, err := first.GetBlockExists(ctx, ref)
	if err != nil || found || second == nil {
		return found, err
	}
	return second.GetBlockExists(ctx, ref)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (o *StoreOverlay) StatBlock(ctx context.Context, ref *BlockRef) (*BlockStat, error) {
	first, second, _ := o.readRoute()
	stat, err := first.StatBlock(ctx, ref)
	if err != nil || stat != nil || second == nil {
		return stat, err
	}
	return second.StatBlock(ctx, ref)
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
// If the hash type is unset, use the type from GetHashType().
func (o *StoreOverlay) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	putOpts, syncRequested := PutOptsWithoutSync(opts)
	finish := func(ref *BlockRef, existed bool, err error) (*BlockRef, bool, error) {
		if err != nil || !syncRequested {
			return ref, existed, err
		}
		if _, syncErr := o.Sync(ctx); syncErr != nil {
			return ref, existed, syncErr
		}
		return ref, existed, nil
	}

	cacheMode := func(s1, s2 StoreOps) (*BlockRef, bool, error) {
		ref, existed, err := s1.PutBlock(ctx, data, putOpts)
		if err != nil {
			return nil, false, err
		}
		lowerOpts := putOpts.CloneVT()
		if lowerOpts == nil {
			lowerOpts = &PutOpts{}
		}
		lowerOpts.ForceBlockRef = ref
		_, lowerExisted, err := s2.PutBlock(ctx, data, lowerOpts)
		if err != nil {
			return nil, false, err
		}
		return ref, existed && lowerExisted, nil
	}

	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		// writes go to the upper store only.
		return finish(o.upper.PutBlock(ctx, data, putOpts))
	case OverlayMode_LOWER_ONLY:
		// writes go to the lower store only.
		return finish(o.lower.PutBlock(ctx, data, putOpts))
	case OverlayMode_UPPER_CACHE:
		// writes go to both stores.
		return finish(cacheMode(o.lower, o.upper))
	case OverlayMode_LOWER_CACHE:
		// writes go to both stores.
		return finish(cacheMode(o.upper, o.lower))
	case OverlayMode_UPPER_READ_CACHE:
		// writes go to the lower store only.
		return finish(o.lower.PutBlock(ctx, data, putOpts))
	case OverlayMode_LOWER_READ_CACHE:
		// writes go to the upper store only.
		return finish(o.upper.PutBlock(ctx, data, putOpts))
	case OverlayMode_UPPER_WRITE_CACHE:
		// writes go to the upper store only.
		return finish(o.upper.PutBlock(ctx, data, putOpts))
	case OverlayMode_LOWER_WRITE_CACHE:
		// writes go to the lower store only.
		return finish(o.lower.PutBlock(ctx, data, putOpts))
	case OverlayMode_UPPER_READBACK_CACHE:
		// writes go to the upper store only (lower is read-only).
		return finish(o.upper.PutBlock(ctx, data, putOpts))
	}
}

// PutBlockBatch writes a batch of blocks using the same target-store policy as
// PutBlock.
func (o *StoreOverlay) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error {
	cacheMode := func(s1, s2 StoreOps) error {
		if err := s1.PutBlockBatch(ctx, entries); err != nil {
			return err
		}
		return s2.PutBlockBatch(ctx, entries)
	}

	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		return o.upper.PutBlockBatch(ctx, entries)
	case OverlayMode_LOWER_ONLY:
		return o.lower.PutBlockBatch(ctx, entries)
	case OverlayMode_UPPER_CACHE:
		return cacheMode(o.lower, o.upper)
	case OverlayMode_LOWER_CACHE:
		return cacheMode(o.upper, o.lower)
	case OverlayMode_UPPER_READ_CACHE:
		return o.lower.PutBlockBatch(ctx, entries)
	case OverlayMode_LOWER_READ_CACHE:
		return o.upper.PutBlockBatch(ctx, entries)
	case OverlayMode_UPPER_WRITE_CACHE:
		return o.upper.PutBlockBatch(ctx, entries)
	case OverlayMode_LOWER_WRITE_CACHE:
		return o.lower.PutBlockBatch(ctx, entries)
	case OverlayMode_UPPER_READBACK_CACHE:
		return o.upper.PutBlockBatch(ctx, entries)
	}
}

// GetBlockExistsBatch checks block existence using the same read policy as GetBlockExists.
func (o *StoreOverlay) GetBlockExistsBatch(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	first, second, _ := o.readRoute()
	out, err := first.GetBlockExistsBatch(ctx, refs)
	if err != nil || second == nil {
		return out, err
	}

	// Ask the second store only for the refs the first store lacks.
	var missing []*BlockRef
	var missingIdx []int
	for i, found := range out {
		if found {
			continue
		}
		missing = append(missing, refs[i])
		missingIdx = append(missingIdx, i)
	}
	if len(missing) == 0 {
		return out, nil
	}
	secondOut, err := second.GetBlockExistsBatch(ctx, missing)
	if err != nil {
		return nil, err
	}
	for i, found := range secondOut {
		out[missingIdx[i]] = found
	}
	return out, nil
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (o *StoreOverlay) RmBlock(ctx context.Context, ref *BlockRef) error {
	cacheMode := func(primary, secondary StoreOps) error {
		uerr := primary.RmBlock(ctx, ref)
		lerr := secondary.RmBlock(ctx, ref)
		if uerr != nil {
			return uerr
		}
		return lerr
	}

	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		// removes go to the upper store only.
		return o.upper.RmBlock(ctx, ref)
	case OverlayMode_LOWER_ONLY:
		// removes go to the lower store only.
		return o.lower.RmBlock(ctx, ref)
	case OverlayMode_UPPER_CACHE:
		// removes go to both stores.
		return cacheMode(o.upper, o.lower)
	case OverlayMode_LOWER_CACHE:
		// removes go to both stores.
		return cacheMode(o.lower, o.upper)
	case OverlayMode_UPPER_READ_CACHE:
		// removes go to both stores.
		return cacheMode(o.lower, o.upper)
	case OverlayMode_LOWER_READ_CACHE:
		// removes go to both stores.
		return cacheMode(o.upper, o.lower)
	case OverlayMode_UPPER_WRITE_CACHE:
		// removes go to the upper (write) store only.
		return o.upper.RmBlock(ctx, ref)
	case OverlayMode_LOWER_WRITE_CACHE:
		// removes go to the lower (write) store only.
		return o.lower.RmBlock(ctx, ref)
	case OverlayMode_UPPER_READBACK_CACHE:
		// removes go to the upper store only; lower lifecycle is external.
		return o.upper.RmBlock(ctx, ref)
	}
}

// Sync fences both overlay stores; it reports fenced only when both did.
func (o *StoreOverlay) Sync(ctx context.Context) (bool, error) {
	upperFenced, err := o.upper.Sync(ctx)
	lowerFenced, lerr := o.lower.Sync(ctx)
	if err == nil {
		err = lerr
	}
	return upperFenced && lowerFenced, err
}

// BeginDeferFlush forwards the GC defer-flush scope to upper and lower stores.
func (o *StoreOverlay) BeginDeferFlush() {
	BeginDeferFlush(o.upper)
	BeginDeferFlush(o.lower)
}

// EndDeferFlush forwards closing the GC defer-flush scope to upper and lower stores.
func (o *StoreOverlay) EndDeferFlush(ctx context.Context) error {
	var err error
	if uerr := EndDeferFlush(ctx, o.upper); uerr != nil {
		err = uerr
	}
	if lerr := EndDeferFlush(ctx, o.lower); lerr != nil && err == nil {
		err = lerr
	}
	return err
}

// _ is a type assertion
var (
	_ StoreOps = (*StoreOverlay)(nil)
)
