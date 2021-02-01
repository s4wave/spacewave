package volume_controller

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_event "github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/golang/protobuf/proto"
)

// defaultHashType is the fallback default hash type
const defaultHashType = hash.HashType_HashType_SHA256

var (
	ErrBucketUnknown = errors.New("bucket not found")
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
	idleWakeCh chan struct{}
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
		idleWakeCh: make(chan struct{}, 1),
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
func (b *bucketHandle) PutBlock(data []byte, opts *bucket.PutOpts) (*bucket_event.PutBlock, error) {
	if b.bucketConf == nil {
		return nil, ErrBucketUnknown
	}
	defer b.startOperation().release()

	hashType := opts.GetHashType()
	if hashType == 0 {
		hashType = b.bucketConf.GetPutOpts().GetHashType()
	}
	if hashType == 0 {
		hashType = defaultHashType
	}

	// hash data
	h, err := hashType.Sum(data)
	if err != nil {
		return nil, err
	}
	br := &cid.BlockRef{
		Hash: hash.NewHash(hashType, h),
	}
	existed, err := b.v.PutBlock(br, data)
	if err != nil {
		return nil, err
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
		ed, err := proto.Marshal(&bucket_event.Event{
			EventType: bucket_event.EventType_EventType_PUT_BLOCK,
			PutBlock:  ev,
		})
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

	return ev, nil
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
func (b *bucketHandle) GetBlock(ref *cid.BlockRef) ([]byte, bool, error) {
	if b.bucketConf == nil {
		return nil, false, ErrBucketUnknown
	}
	defer b.startOperation().release()

	return b.v.GetBlock(ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlockExists.
func (b *bucketHandle) GetBlockExists(ref *cid.BlockRef) (bool, error) {
	if b.bucketConf == nil {
		return false, ErrBucketUnknown
	}
	defer b.startOperation().release()

	return b.v.GetBlockExists(ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (b *bucketHandle) RmBlock(ref *cid.BlockRef) error {
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
		ed, err := proto.Marshal(&bucket_event.Event{
			EventType: bucket_event.EventType_EventType_RM_BLOCK,
			RmBlock:   ev,
		})
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
		if atomic.LoadInt32(&b.nexec) <= 0 {
			return
		}

		<-b.idleWakeCh
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
		select {
		case b.b.idleWakeCh <- struct{}{}:
		default:
		}
	}
}

// _ is a type assertion
var _ bucket.Bucket = ((*bucketHandle)(nil))
