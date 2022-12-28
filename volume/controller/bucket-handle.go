package volume_controller

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_event "github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/broadcast"
)

// bucketHandle implements Bucket with a volume handle.
type bucketHandle struct {
	// nexec is the total number of references + executing calls.
	// atomic integers.
	nexec      int32
	c          *Controller
	baseCtx    context.Context
	ctx        context.Context
	ctxCancel  context.CancelFunc
	v          volume.Volume
	bucketConf *bucket.Config
	idleBcast  broadcast.Broadcast
}

// newBucketHandle builds a new bucket handle
func newBucketHandle(
	ctx context.Context,
	c *Controller,
	v volume.Volume,
	bucketConf *bucket.Config,
) *bucketHandle {
	nctx, nctxCancel := context.WithCancel(ctx)
	return &bucketHandle{
		baseCtx:    ctx,
		ctx:        nctx,
		ctxCancel:  nctxCancel,
		c:          c,
		v:          v,
		bucketConf: bucketConf,
	}
}

// GetContext returns the handle context.
func (b *bucketHandle) GetContext() context.Context {
	return b.ctx
}

// GetID returns the bucket ID.
func (b *bucketHandle) GetID() string {
	return b.bucketConf.GetId()
}

// GetVolumeId returns the volume ID.
func (b *bucketHandle) GetVolumeId() string {
	return b.v.GetID()
}

// GetBucket returns the bucket interface.
func (b *bucketHandle) GetBucket() bucket.Bucket {
	if b.bucketConf == nil {
		return nil
	}

	return b
}

// GetExists indicates if the bucket exists and the handle is valid.
func (b *bucketHandle) GetExists() bool {
	return b.bucketConf != nil
}

// GetBucketConfig returns the bucket configuration.
//
// note: may be nil if this is the pin controller.
func (b *bucketHandle) GetBucketConfig() *bucket.Config {
	return b.bucketConf
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (b *bucketHandle) PutBlock(data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if b.bucketConf == nil {
		return nil, false, volume.ErrBucketUnknown
	}
	defer b.startOperation().release()

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
		err := b.c.pushEventToReconcilers(b.baseCtx, b.v, b.bucketConf, true, getEventData)
		if err != nil {
			b.c.le.
				WithError(err).
				WithField("bucket-id", b.bucketConf.GetId()).
				Warn("unable to push put event to reconcilers")
		}
	}

	return br, false, nil
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
func (b *bucketHandle) GetBlock(ref *block.BlockRef) ([]byte, bool, error) {
	if b.bucketConf == nil {
		return nil, false, volume.ErrBucketUnknown
	}
	defer b.startOperation().release()

	return b.v.GetBlock(ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlockExists.
func (b *bucketHandle) GetBlockExists(ref *block.BlockRef) (bool, error) {
	if b.bucketConf == nil {
		return false, volume.ErrBucketUnknown
	}
	defer b.startOperation().release()

	return b.v.GetBlockExists(ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (b *bucketHandle) RmBlock(ref *block.BlockRef) error {
	if b.bucketConf == nil {
		return nil
	}
	defer b.startOperation().release()

	if !b.c.config.GetDisableEventBlockRm() {
		ok, err := b.v.GetBlockExists(ref)
		if err == nil && !ok {
			// skip, does not exist.
			return nil
		}
	}

	if err := b.v.RmBlock(ref); err != nil || b.c.config.GetDisableEventBlockRm() {
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
	_ = b.c.pushEventToReconcilers(b.baseCtx, b.v, b.bucketConf, true, getEventData)
	return nil
}

// Flush cancels the handle and waits for ongoing requests to exit.
func (b *bucketHandle) Flush() {
	b.c.le.Debug("bucket handle Flush()")
	b.ctxCancel()
	for {
		waitCh := b.idleBcast.GetWaitCh()
		if atomic.LoadInt32(&b.nexec) <= 0 {
			return
		}
		<-waitCh
	}
}

// superceeds checks if the handle superceeds another
func (b *bucketHandle) superceeds(o *bucketHandle) bool {
	if b.bucketConf != nil && o.bucketConf == nil {
		return true
	}
	return b.bucketConf.GetVersion() > o.bucketConf.GetVersion()
}

// bucketHandleOp is a running operation for a bucket handle.
type bucketHandleOp struct {
	b *bucketHandle
}

// startOperation starts a call.
func (b *bucketHandle) startOperation() *bucketHandleOp {
	atomic.AddInt32(&b.nexec, 1)
	return &bucketHandleOp{b: b}
}

// release indicates the op has concluded
func (b *bucketHandleOp) release() {
	if atomic.AddInt32(&b.b.nexec, -1) <= 0 {
		b.b.idleBcast.Broadcast()
	}
}

// _ is a type assertion
var _ bucket.Bucket = ((*bucketHandle)(nil))
