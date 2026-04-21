package bifrost_entitygraph

import (
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/entitygraph/entity"
	"github.com/aperturerobotics/entitygraph/link"
	"github.com/s4wave/spacewave/net/transport"
)

// lookupTransportHandler handles the LookupTransport directive results
type lookupTransportHandler struct {
	c    *Reporter
	mtx  sync.Mutex
	vals map[directive.Value]lookupTransportHandlerVal
}

// lookupTransportHandlerVal is the value tuple
type lookupTransportHandlerVal struct {
	tptObj   entity.Entity
	assocObj link.Link
}

// newLookupTransportHandler constructs a lookupTransportHandler
func newLookupTransportHandler(c *Reporter) *lookupTransportHandler {
	return &lookupTransportHandler{
		c:    c,
		vals: make(map[directive.Value]lookupTransportHandlerVal),
	}
}

// HandleValueAdded is called when a value is added to the directive.
func (h *lookupTransportHandler) HandleValueAdded(
	inst directive.Instance,
	val directive.AttachedValue,
) {
	tpt, ok := val.GetValue().(transport.Transport)
	if !ok {
		h.c.le.Warn("LookupTransport value was not a Transport")
		return
	}

	tptObj, tptAssocObj := NewTransportEntity(tpt.GetUUID(), tpt.GetPeerID())
	h.mtx.Lock()
	_, exists := h.vals[val]
	if !exists {
		h.vals[val] = lookupTransportHandlerVal{
			tptObj:   tptObj,
			assocObj: tptAssocObj,
		}
	}
	h.mtx.Unlock()

	if !exists {
		h.c.store.AddEntityObj(tptObj)
		h.c.store.AddEntityObj((link.Link)(tptAssocObj))
	}
}

// HandleValueRemoved is called when a value is removed from the directive.
func (h *lookupTransportHandler) HandleValueRemoved(
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
		h.c.store.RemoveEntityObj(ent.tptObj)
		h.c.store.RemoveEntityObj(ent.assocObj)
	}
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (h *lookupTransportHandler) HandleInstanceDisposed(inst directive.Instance) {
	// noop
}

// _ is a type assertion
var _ directive.ReferenceHandler = ((*lookupTransportHandler)(nil))
