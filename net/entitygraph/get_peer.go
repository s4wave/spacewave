package bifrost_entitygraph

import (
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/entitygraph/entity"
	"github.com/s4wave/spacewave/net/peer"
)

// getPeerHandler handles the GetPeer directive results
type getPeerHandler struct {
	c    *Reporter
	mtx  sync.Mutex
	vals map[directive.Value]entity.Entity
}

// newGetPeerHandler constructs a getPeerHandler
func newGetPeerHandler(c *Reporter) *getPeerHandler {
	return &getPeerHandler{
		c:    c,
		vals: make(map[directive.Value]entity.Entity),
	}
}

// HandleValueAdded is called when a value is added to the directive.
func (h *getPeerHandler) HandleValueAdded(
	inst directive.Instance,
	val directive.AttachedValue,
) {
	nod, ok := val.GetValue().(peer.Peer)
	if !ok {
		h.c.le.Warn("GetPeer value was not a Peer")
		return
	}

	nodObj := NewPeerEntity(nod.GetPeerID())
	h.mtx.Lock()
	_, exists := h.vals[val]
	if !exists {
		h.vals[val] = nodObj
	}
	h.mtx.Unlock()

	if !exists {
		h.c.store.AddEntityObj(nodObj)
	}
}

// HandleValueRemoved is called when a value is removed from the directive.
func (h *getPeerHandler) HandleValueRemoved(
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
		h.c.store.RemoveEntityObj(ent)
	}
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (h *getPeerHandler) HandleInstanceDisposed(inst directive.Instance) {
	// noop
}

// _ is a type assertion
var _ directive.ReferenceHandler = ((*getPeerHandler)(nil))
