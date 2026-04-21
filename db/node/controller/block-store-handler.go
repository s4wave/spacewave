package node_controller

import (
	"github.com/aperturerobotics/controllerbus/directive"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// blockStoreRefHandler implements the reference handler for LookupBlockStore.
type blockStoreRefHandler struct {
	c *Controller
}

func newBlockStoreRefHandler(c *Controller) *blockStoreRefHandler {
	return &blockStoreRefHandler{c: c}
}

// HandleValueAdded is called when a value is added to the directive.
// Should not block.
func (r *blockStoreRefHandler) HandleValueAdded(
	i directive.Instance,
	av directive.AttachedValue,
) {
	v, ok := av.GetValue().(block_store.Store)
	if !ok {
		r.c.le.Warn("ignoring invalid value for LookupBlockStore")
		return
	}

	blockStoreID := v.GetID()
	if blockStoreID == "" {
		// this should not happen
		r.c.le.Warn("ignoring value for LookupBlockStore with empty block store id")
		return
	}

	r.c.mtx.Lock()
	if vb, ok := r.c.blockStores[blockStoreID]; !ok || vb != v {
		r.c.le.WithField("block-store-id", blockStoreID).Debug("block store added")
		r.c.blockStores[blockStoreID] = v
		bkts := r.c.buckets.GetKeysWithData()
		for _, b := range bkts {
			b.Data.PushBlockStore(blockStoreID, true)
		}
	}
	r.c.mtx.Unlock()
}

// HandleValueRemoved is called when a value is removed from the directive.
// Should not block.
func (r *blockStoreRefHandler) HandleValueRemoved(
	i directive.Instance,
	av directive.AttachedValue,
) {
	v, ok := av.GetValue().(block_store.Store)
	if !ok {
		return
	}
	blockStoreID := v.GetID()
	if blockStoreID == "" {
		return
	}
	r.c.mtx.Lock()
	if vb, ok := r.c.blockStores[blockStoreID]; ok && vb == v {
		r.c.le.WithField("block-store-id", blockStoreID).Debug("block store removed")
		delete(r.c.blockStores, blockStoreID)
		bkts := r.c.buckets.GetKeysWithData()
		for _, b := range bkts {
			b.Data.ClearBlockStore(blockStoreID)
		}
	}
	r.c.mtx.Unlock()
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (r *blockStoreRefHandler) HandleInstanceDisposed(i directive.Instance) {
	r.c.mtx.Lock()
	bkts := r.c.buckets.GetKeysWithData()
	for k := range r.c.blockStores {
		delete(r.c.blockStores, k)
		for _, b := range bkts {
			b.Data.ClearBlockStore(k)
		}
	}
	r.c.mtx.Unlock()
}

// _ is a type assertion
var _ directive.ReferenceHandler = ((*blockStoreRefHandler)(nil))
