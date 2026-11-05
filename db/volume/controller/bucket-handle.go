package volume_controller

import (
	"context"
	"errors"
	"slices"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/hash"
)

// bucketHandleTracker implements Bucket with a volume handle.
type bucketHandleTracker struct {
	// c resolves the volume and bucket configuration.
	c *Controller
	// bucketID identifies the tracked bucket.
	bucketID string
	// handleCtr publishes the current bucket handle or resolution error.
	handleCtr *ccontainer.CContainer[*bucketHandle]
}

// bucketHandle contains state resolved by the bucket handle tracker.
type bucketHandle struct {
	// t tracks this bucket's configuration and lifetime.
	t *bucketHandleTracker
	// err records a failure resolving the bucket.
	err error
	// v provides the backing volume.
	v volume.Volume
	// bucketConf retains the resolved bucket configuration.
	bucketConf *bucket.Config
	// readOps confines block operations to an existing read snapshot when set.
	readOps block.StoreOps
	// stage is the volume stage that owns this handle's writes, if any.
	stage string
}

// clone copies the bucket handle without changing its retained dependencies.
func (b *bucketHandle) clone() *bucketHandle {
	if b == nil {
		return b
	}
	x := *b
	return &x
}

// newBucketHandleTracker builds a new bucket handle tracker.
func (c *Controller) newBucketHandleTracker(
	bucketID string,
) (keyed.Routine, *bucketHandleTracker) {
	h := &bucketHandleTracker{
		c:         c,
		bucketID:  bucketID,
		handleCtr: ccontainer.NewCContainer[*bucketHandle](nil),
	}
	return h.execute, h
}

// execute executes the bucket handle management routine.
func (b *bucketHandleTracker) execute(ctx context.Context) (exErr error) {
	// Clear stale resolution and publish any new failure when this attempt ends.
	b.handleCtr.SetValue(nil)
	defer func() {
		if exErr != nil {
			if exErr == context.Canceled {
				b.handleCtr.SetValue(nil)
			} else {
				b.handleCtr.SetValue(&bucketHandle{t: b, err: exErr})
			}
		}
	}()

	// Resolve the backing volume and its current bucket configuration.
	vol, err := b.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	// Load the bucket configuration before constructing its handle.
	bc, err := vol.GetBucketConfig(ctx, b.bucketID)
	if err != nil {
		return err
	}

	// Publish the handle, which retains the resolved configuration
	// independently of later updates.
	b.handleCtr.SetValue(&bucketHandle{
		t:          b,
		v:          vol,
		bucketConf: bc,
	})
	return nil
}

// updateBucketConfig overrides the bucket config in the current handle.
//
// A nil configuration clears the handle and restarts resolution.
// An existing handle receives the configuration; otherwise resolution restarts.
func (b *bucketHandleTracker) updateBucketConfig(conf *bucket.Config) *bucketHandle {
	// Restart resolution when the caller invalidates the configuration.
	if conf == nil {
		b.handleCtr.SetValue(nil)
		b.restart()
		return nil
	}

	// Update a resolved handle without mutating its previous snapshot.
	conf = conf.CloneVT()
	handle := b.handleCtr.SwapValue(func(val *bucketHandle) *bucketHandle {
		if val == nil || val.bucketConf.EqualVT(conf) {
			return val
		}
		val = val.clone()
		val.bucketConf = conf
		return val
	})
	if handle != nil {
		return handle
	}
	b.restart()
	return nil
}

// restart restarts the routine.
func (b *bucketHandleTracker) restart() {
	_, _ = b.c.bucketHandles.RestartRoutine(b.bucketID)
}

// GetID returns the bucket ID.
func (b *bucketHandle) GetID() string {
	return b.t.bucketID
}

// GetVolumeId returns the volume ID.
func (b *bucketHandle) GetVolumeId() string {
	return b.v.GetID()
}

// GetBucket returns the bucket interface.
func (b *bucketHandle) GetBucket() bucket.Bucket {
	if !b.GetExists() {
		return nil
	}

	return b
}

// GetExists indicates if the bucket exists.
func (b *bucketHandle) GetExists() bool {
	return b.bucketConf.GetId() != ""
}

// GetBucketConfig returns the bucket configuration.
//
// The configuration may be nil for the pin controller.
func (b *bucketHandle) GetBucketConfig() *bucket.Config {
	if !b.GetExists() {
		return nil
	}
	return b.bucketConf
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (b *bucketHandle) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Trace the complete bucket write.
	ctx, task := trace.NewTask(ctx, "hydra/volume/bucket-handle/put-block")
	defer task.End()

	// Reject writes on a failed or missing bucket and honor read scopes.
	if b.err != nil {
		return nil, false, b.err
	}
	if b.bucketConf == nil {
		return nil, false, bucket.ErrBucketNotFound
	}
	if b.readOps != nil {
		return b.readOps.PutBlock(ctx, data, opts)
	}

	// Fill the bucket's preferred hash type when the caller leaves it unset.
	if opts.GetHashType() == 0 {
		ht := opts.GetForceBlockRef().GetHash().GetHashType()
		if ht == 0 {
			ht = b.GetHashType()
		}
		if ht != 0 {
			if opts == nil {
				opts = &block.PutOpts{}
			} else {
				opts = opts.CloneVT()
			}
			opts.HashType = ht
		}
	}
	putOpts, syncRequested := block.PutOptsWithoutSync(opts)

	// A stage owns the writes of a staged handle.
	if b.stage != "" {
		ref, existed, err := b.v.PrepareStagedBlock(ctx, b.stage, data, putOpts)
		if err == nil && syncRequested {
			_, err = b.Sync(ctx)
		}
		return ref, existed, err
	}

	// Store the block with its child list in one physical transaction when
	// the volume supports it, preserving PutBlock's existence result.
	if prepare, ok := b.v.(block.AtomicBlockPreparer); ok && b.preparesOwned() {
		ref, existed, err := prepare.PrepareOwnedBlock(ctx, b.GetID(), data, putOpts)
		if !errors.Is(err, block.ErrAtomicCommitUnsupported) {
			if err == nil && syncRequested {
				_, err = b.Sync(ctx)
			}
			return ref, existed, err
		}
	}

	// Write through the volume otherwise.
	br, existed, err := b.v.PutBlock(ctx, data, putOpts)
	if err != nil {
		return nil, false, err
	}
	if syncRequested {
		if _, err := b.Sync(ctx); err != nil {
			return br, existed, err
		}
	}

	return br, existed, nil
}

// PutBlockBatch writes a batch of blocks through the bucket in a single
// lower-layer operation.
func (b *bucketHandle) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Trace the complete bucket batch write.
	ctx, task := trace.NewTask(ctx, "hydra/volume/bucket-handle/put-block-batch")
	defer task.End()

	// Reject writes on a failed or missing bucket and honor read scopes.
	if b.err != nil {
		return nil, b.err
	}
	if b.bucketConf == nil {
		return nil, bucket.ErrBucketNotFound
	}
	if len(entries) == 0 {
		return nil, nil
	}
	if b.readOps != nil {
		return b.readOps.PutBlockBatch(ctx, entries)
	}

	// On a volume that keeps liveness a tombstone releases what the bucket's
	// write held, and never evicts bytes another holder may reach: a stage
	// releases its root on the block, and any other put held nothing.
	if b.stage != "" || b.SupportsRootRetention() {
		return b.putReleasing(ctx, entries)
	}
	return b.v.PutBlockBatch(ctx, entries)
}

// putReleasing stores the batch's blocks on a volume that keeps liveness, and
// releases the stage roots its tombstones name. It reports a tombstone as not
// stored before.
func (b *bucketHandle) putReleasing(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Split the tombstones from the puts.
	puts := entries
	var released []*block.BlockRef
	if slices.ContainsFunc(entries, isTombstone) {
		puts = make([]*block.PutBatchEntry, 0, len(entries))
		for _, entry := range entries {
			if isTombstone(entry) {
				released = append(released, entry.Ref)
				continue
			}
			puts = append(puts, entry)
		}
	}

	// Store the puts under the stage or as the bucket's writes.
	vol := b.v
	var stored []bool
	var err error
	switch {
	case len(puts) == 0:
	case b.stage != "":
		stored, err = vol.PrepareStagedBlockBatch(ctx, b.stage, puts)
	default:
		stored, err = b.putOwned(ctx, puts)
	}
	if err != nil {
		return nil, err
	}

	// Release the stage's roots on the tombstoned blocks.
	if b.stage != "" && len(released) != 0 {
		if err := vol.ReleaseStageRoots(ctx, b.stage, released); err != nil {
			return nil, err
		}
	}
	if len(puts) == len(entries) {
		return stored, nil
	}

	// Report each entry's result in the caller's order.
	existed := make([]bool, len(entries))
	for i, j := 0, 0; i < len(entries); i++ {
		if !isTombstone(entries[i]) {
			existed[i] = stored[j]
			j++
		}
	}
	return existed, nil
}

// putOwned stores puts as the bucket's writes, in one physical transaction
// when the volume prepares owned batches.
func (b *bucketHandle) putOwned(ctx context.Context, puts []*block.PutBatchEntry) ([]bool, error) {
	if prepare, ok := b.v.(block.AtomicBlockPreparer); ok {
		return prepare.PrepareOwnedBlockBatch(ctx, b.GetID(), puts)
	}
	return b.v.PutBlockBatch(ctx, puts)
}

// isTombstone reports whether entry deletes its block.
func isTombstone(entry *block.PutBatchEntry) bool {
	return entry != nil && entry.Tombstone
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (b *bucketHandle) GetHashType() hash.HashType {
	if b != nil && b.readOps != nil {
		return b.readOps.GetHashType()
	}
	if b != nil && b.v != nil {
		return b.v.GetHashType()
	}
	return 0
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (b *bucketHandle) GetSupportedFeatures() block.StoreFeature {
	// Report unknown features without a backing volume.
	if b == nil || b.v == nil {
		return block.StoreFeature_STORE_FEATURE_UNKNOWN
	}
	if b.readOps != nil {
		return b.readOps.GetSupportedFeatures()
	}
	return b.v.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope for the bucket's volume reads.
func (b *bucketHandle) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// Select the complete wrapper chain before opening its one read scope.
	if b == nil || b.v == nil {
		return b, func() {}, nil
	}
	store := block.StoreOps(b.v)
	if b.readOps != nil {
		store = b.readOps
	}
	scopedOps, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Keep the bucket's metadata while all block operations use the scoped store.
	scoped := b.clone()
	scoped.readOps = scopedOps
	if v, ok := scopedOps.(volume.Volume); ok {
		scoped.v = v
	}
	return scoped, release, nil
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
func (b *bucketHandle) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if b.bucketConf == nil {
		return nil, false, bucket.ErrBucketNotFound
	}
	if b.readOps != nil {
		return b.readOps.GetBlock(ctx, ref)
	}

	return b.v.GetBlock(ctx, ref)
}

// GetStoredBlock gets a block and its child list, which the volume keeps.
func (b *bucketHandle) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if b.bucketConf == nil {
		return nil, bucket.ErrBucketNotFound
	}
	if b.readOps != nil {
		return b.readOps.GetStoredBlock(ctx, ref)
	}
	return b.v.GetStoredBlock(ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlockExists.
func (b *bucketHandle) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if b.bucketConf == nil {
		return false, bucket.ErrBucketNotFound
	}
	if b.readOps != nil {
		return b.readOps.GetBlockExists(ctx, ref)
	}

	return b.v.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks if blocks exist.
func (b *bucketHandle) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if b.bucketConf == nil {
		return nil, bucket.ErrBucketNotFound
	}
	if b.readOps != nil {
		return b.readOps.GetBlockExistsBatch(ctx, refs)
	}
	return b.v.GetBlockExistsBatch(ctx, refs)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (b *bucketHandle) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if b.bucketConf == nil {
		return nil, bucket.ErrBucketNotFound
	}
	if b.readOps != nil {
		return b.readOps.StatBlock(ctx, ref)
	}
	return b.v.StatBlock(ctx, ref)
}

// RmBlock releases the bucket's hold on the block through the batch mutation
// path. On a volume that keeps liveness that releases a stage's root and
// otherwise nothing, since a put holds nothing; only the collector frees.
// Removing an absent block succeeds.
func (b *bucketHandle) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if b.bucketConf == nil {
		return nil
	}
	if b.readOps != nil {
		return b.readOps.RmBlock(ctx, ref)
	}

	_, err := b.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
	return err
}

// Sync fences the volume.
func (b *bucketHandle) Sync(ctx context.Context) (bool, error) {
	if b.readOps != nil {
		return b.readOps.Sync(ctx)
	}
	return b.v.Sync(ctx)
}

// preparesOwned reports whether the handle stores a block with its bucket
// ownership in one Volume transaction: its normal write domain, not a
// transient read scope, on a Volume that keeps liveness.
func (b *bucketHandle) preparesOwned() bool {
	if b.err != nil || b.readOps != nil || b.v == nil {
		return false
	}
	return block.SupportsRootRetention(b.v)
}

// _ asserts the bucket contracts.
var (
	_ bucket.Bucket       = (*bucketHandle)(nil)
	_ bucket.BucketHandle = (*bucketHandle)(nil)
)
