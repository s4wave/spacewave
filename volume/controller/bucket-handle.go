package volume_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_gc "github.com/aperturerobotics/hydra/block/gc"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_event "github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
)

// bucketHandleTracker implements Bucket with a volume handle.
type bucketHandleTracker struct {
	c         *Controller
	bucketID  string
	handleCtr *ccontainer.CContainer[*bucketHandle]
}

// bucketHandle contains state resolved by the bucket handle tracker.
type bucketHandle struct {
	t          *bucketHandleTracker
	err        error
	v          volume.Volume
	bucketConf *bucket.Config
	gcOps      *block_gc.GCStoreOps
}

// clone copies the bucketHandle
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

	vol, err := b.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	bc, err := vol.GetBucketConfig(ctx, b.bucketID)
	if err != nil {
		return err
	}

	handle := &bucketHandle{
		t:          b,
		v:          vol,
		bucketConf: bc,
	}

	// Wrap block operations with GC tracking if the volume has a RefGraph.
	if rg := vol.GetRefGraph(); rg != nil {
		handle.gcOps = block_gc.NewGCStoreOpsWithParent(
			vol,
			rg,
			block_gc.BucketIRI(b.bucketID),
		)
	}

	b.handleCtr.SetValue(handle)

	return nil
}

// updateBucketConfig overrides the bucket config in the current handle.
//
// if conf is nil, unsets the handle ctr and restarts the routine.
// if there is a current handle set: returns the updated bucket handle.
// if there is no handle set: restarts the routine and returns nil.
func (b *bucketHandleTracker) updateBucketConfig(conf *bucket.Config) *bucketHandle {
	if conf == nil {
		b.handleCtr.SetValue(nil)
		b.restart()
		return nil
	}

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
// note: may be nil if this is the pin controller.
func (b *bucketHandle) GetBucketConfig() *bucket.Config {
	if !b.GetExists() {
		return nil
	}
	return b.bucketConf
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (b *bucketHandle) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if b.err != nil {
		return nil, false, b.err
	}
	if b.bucketConf == nil {
		return nil, false, bucket.ErrBucketNotFound
	}

	// set hash type if not set
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

	// store will hash the data, route through GCStoreOps if available
	var (
		br      *block.BlockRef
		existed bool
		err     error
	)
	if b.gcOps != nil {
		br, existed, err = b.gcOps.PutBlock(ctx, data, opts)
		if err != nil {
			return nil, false, err
		}
		if err := b.gcOps.FlushPending(ctx); err != nil {
			return nil, false, err
		}
	} else {
		br, existed, err = b.v.PutBlock(ctx, data, opts)
	}
	if err != nil {
		return nil, false, err
	}

	var eventData []byte
	ev := &bucket_event.PutBlock{
		BlockCommon: &bucket_event.BlockCommon{
			VolumeId:      b.v.GetID(),
			BucketId:      b.bucketConf.GetId(),
			BucketConfRev: b.bucketConf.GetRev(),
			BlockRef:      br,
		},
	}

	// wake reconcilers
	if !existed {
		err := b.t.c.pushEventToReconcilers(ctx, b.v, b.bucketConf, true, func() ([]byte, error) {
			if eventData != nil {
				return eventData, nil
			}
			ed, err := (&bucket_event.Event{
				EventType: bucket_event.EventType_EventType_PUT_BLOCK,
				PutBlock:  ev,
			}).MarshalVT()
			if err != nil {
				return nil, err
			}
			eventData = ed
			return ed, nil
		})
		if err != nil {
			b.t.c.le.
				WithError(err).
				WithField("bucket-id", b.bucketConf.GetId()).
				Warn("unable to push put event to reconcilers")
		}
	}

	return br, existed, nil
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (b *bucketHandle) GetHashType() hash.HashType {
	if b != nil && b.v != nil {
		return b.v.GetHashType()
	}
	return 0
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
func (b *bucketHandle) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if b.bucketConf == nil {
		return nil, false, bucket.ErrBucketNotFound
	}

	return b.v.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlockExists.
func (b *bucketHandle) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if b.bucketConf == nil {
		return false, bucket.ErrBucketNotFound
	}

	return b.v.GetBlockExists(ctx, ref)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (b *bucketHandle) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if b.bucketConf == nil {
		return nil, bucket.ErrBucketNotFound
	}

	if b.gcOps != nil {
		return b.gcOps.StatBlock(ctx, ref)
	}
	return b.v.StatBlock(ctx, ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (b *bucketHandle) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if b.bucketConf == nil {
		return nil
	}

	if !b.t.c.config.GetDisableEventBlockRm() {
		ok, err := b.v.GetBlockExists(ctx, ref)
		if err == nil && !ok {
			// skip, does not exist.
			return nil
		}
	}

	// Route through GCStoreOps if available for ref graph cleanup.
	var rmErr error
	if b.gcOps != nil {
		rmErr = b.gcOps.RmBlock(ctx, ref)
	} else {
		rmErr = b.v.RmBlock(ctx, ref)
	}
	if rmErr != nil || b.t.c.config.GetDisableEventBlockRm() {
		return rmErr
	}

	var eventData []byte
	ev := &bucket_event.RmBlock{
		BlockCommon: &bucket_event.BlockCommon{
			VolumeId:      b.v.GetID(),
			BucketId:      b.bucketConf.GetId(),
			BucketConfRev: b.bucketConf.GetRev(),
			BlockRef:      ref,
		},
	}
	getEventData := func() ([]byte, error) {
		if eventData != nil {
			return eventData, nil
		}
		ed, err := (&bucket_event.Event{
			EventType: bucket_event.EventType_EventType_RM_BLOCK,
			RmBlock:   ev,
		}).MarshalVT()
		if err != nil {
			return nil, err
		}
		eventData = ed
		return ed, nil
	}

	// wake reconcilers
	_ = b.t.c.pushEventToReconcilers(ctx, b.v, b.bucketConf, true, getEventData)
	return nil
}

// _ is a type assertion
var (
	_ bucket.Bucket       = ((*bucketHandle)(nil))
	_ bucket.BucketHandle = ((*bucketHandle)(nil))
)
