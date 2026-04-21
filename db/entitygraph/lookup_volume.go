package hydra_entitygraph

import (
	"sync"

	"github.com/s4wave/spacewave/db/volume"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/entitygraph/entity"
)

// lookupVolumeHandler handles the lookupVolume directive results
type lookupVolumeHandler struct {
	c    *Reporter
	mtx  sync.Mutex
	vals map[directive.Value]lookupVolumeHandlerVal
}

// lookupVolumeHandlerVal is the value tuple
type lookupVolumeHandlerVal struct {
	volObj entity.Entity
}

// newLookupVolumeHandler constructs a lookupVolumeHandler
func newLookupVolumeHandler(c *Reporter) *lookupVolumeHandler {
	return &lookupVolumeHandler{
		c:    c,
		vals: make(map[directive.Value]lookupVolumeHandlerVal),
	}
}

// HandleValueAdded is called when a value is added to the directive.
func (h *lookupVolumeHandler) HandleValueAdded(
	inst directive.Instance,
	val directive.AttachedValue,
) {
	vol, ok := val.GetValue().(volume.Volume)
	if !ok {
		h.c.le.Warn("lookupVolume value was not a volume")
		return
	}

	volObj := NewVolumeEntity(vol)
	h.mtx.Lock()
	_, exists := h.vals[val]
	if !exists {
		h.vals[val] = lookupVolumeHandlerVal{
			volObj: volObj,
		}
	}
	h.mtx.Unlock()

	if !exists {
		h.c.store.AddEntityObj(volObj)
	}
}

// HandleValueRemoved is called when a value is removed from the directive.
func (h *lookupVolumeHandler) HandleValueRemoved(
	inst directive.Instance,
	val directive.AttachedValue,
) {
	h.mtx.Lock()
	ent, exists := h.vals[val]
	if exists {
		delete(h.vals, val)
	}
	h.mtx.Unlock()

	if exists {
		h.c.store.RemoveEntityObj(ent.volObj)
	}
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (h *lookupVolumeHandler) HandleInstanceDisposed(inst directive.Instance) {
	// noop
}

// _ is a type assertion
var _ directive.ReferenceHandler = ((*lookupVolumeHandler)(nil))
