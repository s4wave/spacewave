package volume

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/object"
)

// BusObjectStore implements ObjectStore backed by a volume on a bus.
//
// Executes BuildObjectStore directive to get an object store handle.
type BusObjectStore struct {
	ctx           context.Context
	b             bus.Bus
	returnIfIdle  bool
	storeID       string
	storeVolumeID string
}

// NewBusObjectStore constructs a new BusObjectStore.
//
// if returnIfIdle is set and the directive becomes idle, returns ErrObjectStoreUnavailable.
func NewBusObjectStore(ctx context.Context, b bus.Bus, returnIfIdle bool, storeID, storeVolumeID string) *BusObjectStore {
	return &BusObjectStore{ctx: ctx, b: b, returnIfIdle: returnIfIdle, storeID: storeID, storeVolumeID: storeVolumeID}
}

// NewTransaction returns a new transaction against the store.
// Always call Discard() after you are done with the transaction.
// The transaction will be read-only unless write is set.
func (b *BusObjectStore) NewTransaction(write bool) (kvtx.Tx, error) {
	subCtx, subCtxCancel := context.WithCancel(b.ctx)
	var tx atomic.Pointer[busObjectStoreTx]
	handle, ref, err := b.BuildObjectStore(subCtx, func() {
		subCtxCancel()
		ttx := tx.Load()
		if ttx != nil {
			(*ttx).Discard()
		}
	})
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	store := handle.GetObjectStore()
	utx, err := store.NewTransaction(write)
	if err != nil {
		subCtxCancel()
		ref.Release()
		return nil, err
	}
	btx := &busObjectStoreTx{ctx: subCtx, cancel: subCtxCancel, ref: ref, utx: utx}
	tx.Store(btx)
	return btx, nil
}

// BuildObjectStore opens the handle to the object store api.
//
// May return nil, nil, nil, if returnIfIdle is set.
func (b *BusObjectStore) BuildObjectStore(ctx context.Context, disposeCb func()) (BuildObjectStoreAPIValue, directive.Reference, error) {
	val, _, ref, err := BuildObjectStoreAPIEx(ctx, b.b, b.returnIfIdle, b.storeID, b.storeVolumeID, nil)
	return val, ref, err
}

// _ is a type assertion
var _ object.ObjectStore = ((*BusObjectStore)(nil))
