package volume_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
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
	ctx        context.Context
	err        error
	v          volume.Volume
	volID      string
	bucketConf *bucket.Config
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
				b.handleCtr.SetValue(&bucketHandle{t: b, ctx: ctx, err: exErr})
			}
		}
	}()

	vol, err := b.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	bc, err := vol.GetBucketConfig(b.bucketID)
	if err != nil {
		return err
	}

	b.handleCtr.SetValue(&bucketHandle{
		t:          b,
		ctx:        ctx,
		v:          vol,
		volID:      vol.GetID(),
		bucketConf: bc,
	})

	return nil
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
func (b *bucketHandle) PutBlock(data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if b.err != nil {
		return nil, false, b.err
	}
	if b.bucketConf == nil {
		return nil, false, volume.ErrBucketUnknown
	}

	hashType := opts.GetHashType()
	if hashType == 0 {
		opts = b.bucketConf.GetPutOpts()
	}

	// store will hash the data
	br, existed, err := b.v.PutBlock(data, opts)
	if err != nil {
		return nil, false, err
	}

	var eventData []byte
	ev := &bucket_event.PutBlock{
		BlockCommon: &bucket_event.BlockCommon{
			VolumeId:      b.v.GetID(),
			BucketId:      b.bucketConf.GetId(),
			BucketConfRev: b.bucketConf.GetVersion(),
			BlockRef:      br,
		},
	}
	getEventData := func() ([]byte, error) {
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
	}

	// wake reconcilers
	if !existed {
		err := b.t.c.pushEventToReconcilers(b.ctx, b.v, b.bucketConf, true, getEventData)
		if err != nil {
			b.t.c.le.
				WithError(err).
				WithField("bucket-id", b.bucketConf.GetId()).
				Warn("unable to push put event to reconcilers")
		}
	}

	return br, false, nil
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
func (b *bucketHandle) GetBlock(ref *block.BlockRef) ([]byte, bool, error) {
	if b.bucketConf == nil {
		return nil, false, volume.ErrBucketUnknown
	}

	return b.v.GetBlock(ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlockExists.
func (b *bucketHandle) GetBlockExists(ref *block.BlockRef) (bool, error) {
	if b.bucketConf == nil {
		return false, volume.ErrBucketUnknown
	}

	return b.v.GetBlockExists(ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (b *bucketHandle) RmBlock(ref *block.BlockRef) error {
	if b.bucketConf == nil {
		return nil
	}

	if !b.t.c.config.GetDisableEventBlockRm() {
		ok, err := b.v.GetBlockExists(ref)
		if err == nil && !ok {
			// skip, does not exist.
			return nil
		}
	}

	if err := b.v.RmBlock(ref); err != nil || b.t.c.config.GetDisableEventBlockRm() {
		return err
	}

	var eventData []byte
	ev := &bucket_event.RmBlock{
		BlockCommon: &bucket_event.BlockCommon{
			VolumeId:      b.v.GetID(),
			BucketId:      b.bucketConf.GetId(),
			BucketConfRev: b.bucketConf.GetVersion(),
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
	_ = b.t.c.pushEventToReconcilers(b.ctx, b.v, b.bucketConf, true, getEventData)
	return nil
}

// _ is a type assertion
var (
	_ bucket.Bucket       = ((*bucketHandle)(nil))
	_ volume.BucketHandle = ((*bucketHandle)(nil))
)
