package volume_controller

import (
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	"github.com/s4wave/spacewave/db/mqueue"
	"github.com/s4wave/spacewave/db/reconciler"
	"github.com/s4wave/spacewave/db/volume"
)

// reconcilerHandle is a handle passed to a reconciler.
type reconcilerHandle struct {
	pair         bucket_store.BucketReconcilerPair
	bucketHandle bucket.BucketHandle
	vol          volume.Volume
	eveQueue     mqueue.Queue
}

// newReconcilerHandle builds a new reconciler handle.
func newReconcilerHandle(
	pair bucket_store.BucketReconcilerPair,
	bucketHandle bucket.BucketHandle,
	vol volume.Volume,
	eveQueue mqueue.Queue,
) *reconcilerHandle {
	return &reconcilerHandle{
		pair:         pair,
		bucketHandle: bucketHandle,
		vol:          vol,
		eveQueue:     eveQueue,
	}
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
func (h *reconcilerHandle) GetBucketHandle() bucket.BucketHandle {
	return h.bucketHandle
}

// GetBlockStore returns the block store.
func (h *reconcilerHandle) GetBlockStore() block_store.Store {
	return h.vol
}

// GetVolume returns the volume.
func (h *reconcilerHandle) GetVolume() volume.Volume {
	return h.vol
}

// GetEventQueue returns the reconciler event queue handle.
func (h *reconcilerHandle) GetEventQueue() mqueue.Queue {
	return h.eveQueue
}

// _ is a type assertion
var _ reconciler.Handle = ((*reconcilerHandle)(nil))
