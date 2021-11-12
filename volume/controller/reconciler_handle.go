package volume_controller

import (
	"context"

	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/hydra/reconciler"
	"github.com/aperturerobotics/hydra/volume"
)

// reconcilerHandle is a handle passed to a reconciler.
type reconcilerHandle struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	pair         bucket_store.BucketReconcilerPair
	bucketHandle volume.BucketHandle
	vol          volume.Volume
	eveQueue     mqueue.Queue
}

// newReconcilerHandle builds a new reconciler handle.
func newReconcilerHandle(
	ctx context.Context,
	ctxCancel context.CancelFunc,
	pair bucket_store.BucketReconcilerPair,
	bucketHandle volume.BucketHandle,
	vol volume.Volume,
	eveQueue mqueue.Queue,
) *reconcilerHandle {
	return &reconcilerHandle{
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		pair:         pair,
		bucketHandle: bucketHandle,
		vol:          vol,
		eveQueue:     eveQueue,
	}
}

// GetContext returns the context for the handle.
func (h *reconcilerHandle) GetContext() context.Context {
	return h.ctx
}

// GetBucketId returns the bucket id.
func (h *reconcilerHandle) GetBucketId() string {
	return h.pair.BucketID
}

// GetReconcilerId returns the reconciler id.
func (h *reconcilerHandle) GetReconcilerId() string {
	return h.pair.ReconcilerID
}

// GetBucketHandle returns the handle to the bucket.
func (h *reconcilerHandle) GetBucketHandle() volume.BucketHandle {
	return h.bucketHandle
}

// GetVolume returns the volume.
func (h *reconcilerHandle) GetVolume() volume.Volume {
	return h.vol
}

// GetEventQueue returns the reconciler event queue handle.
func (h *reconcilerHandle) GetEventQueue() mqueue.Queue {
	return h.eveQueue
}

// FlushReconciler is called when the reconciler exits.
func (h *reconcilerHandle) FlushReconciler() {
	h.ctxCancel()
}

// _ is a type assertion
var _ reconciler.Handle = ((*reconcilerHandle)(nil))
