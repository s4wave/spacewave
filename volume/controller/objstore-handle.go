package volume_controller

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/hydra/volume"
)

// objectStoreHandle implements ObjectStore with a volume handle.
type objectStoreHandle struct {
	// nexec is the total number of references + executing calls.
	// atomic integers.
	nexec     int32
	c         *Controller
	ctx       context.Context
	ctxCancel context.CancelFunc
	v         volume.Volume
	storeID   string
	objStore  object.ObjectStore
	err       error
}

// newObjectStoreHandle builds a new object store handle
func newObjectStoreHandle(
	ctx context.Context,
	c *Controller,
	v volume.Volume,
	objStore object.ObjectStore,
	err error,
	storeID string,
) *objectStoreHandle {
	nctx, nctxCancel := context.WithCancel(ctx)
	return &objectStoreHandle{
		c:         c,
		v:         v,
		err:       err,
		ctx:       nctx,
		storeID:   storeID,
		objStore:  objStore,
		ctxCancel: nctxCancel,
	}
}

// GetContext returns the handle context.
func (b *objectStoreHandle) GetContext() context.Context {
	return b.ctx
}

// GetID returns the store ID.
func (b *objectStoreHandle) GetID() string {
	return b.storeID
}

// GetVolumeId returns the volume ID.
func (b *objectStoreHandle) GetVolumeId() string {
	return b.v.GetID()
}

// GetObjectStore returns the object store interface.
func (b *objectStoreHandle) GetObjectStore() object.ObjectStore {
	return b.objStore
}

// GetError returns any error getting the store.
func (b *objectStoreHandle) GetError() error {
	return b.err
}

// GetNexec returns a snapshot of the number of references.
func (b *objectStoreHandle) GetNexec() int {
	// staticcheck fix
	_ = b.nexec
	return int(atomic.LoadInt32(&b.nexec))
}

// _ is a type assertion
var _ volume.ObjectStoreHandle = ((*objectStoreHandle)(nil))
