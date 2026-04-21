package pubsub_controller

import (
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/pubsub"
)

// establishLinkHandler handles EstablishLink values
type establishLinkHandler struct {
	// c is the controller
	c *Controller
	// ref is the reference
	ref directive.Reference
}

// newEstablishLinkHandler constructs a new establishLinkHandler
func newEstablishLinkHandler(c *Controller) *establishLinkHandler {
	return &establishLinkHandler{
		c: c,
	}
}

// handleEstablishLink handles an EstablishLink directive.
func (c *Controller) handleEstablishLink(di directive.Instance) {
	handler := newEstablishLinkHandler(c)
	ref := di.AddReference(handler, true)
	if ref == nil {
		return
	}
	handler.ref = ref
	c.cleanupRefs = append(c.cleanupRefs, ref)
}

// HandleValueAdded is called when a value is added to the directive.
func (e *establishLinkHandler) HandleValueAdded(inst directive.Instance, val directive.AttachedValue) {
	vl, ok := val.GetValue().(link.MountedLink)
	if !ok {
		e.c.le.Warn("EstablishLink value was not a Link")
		return
	}
	e.c.le.Debugf("got link with uuid %v", vl.GetLinkUUID())

	// Attempt to open the stream.
	e.c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		e.c.incLinks = append(e.c.incLinks, vl)
		broadcast()
	})
}

// HandleValueRemoved is called when a value is removed from the directive.
func (e *establishLinkHandler) HandleValueRemoved(inst directive.Instance, val directive.AttachedValue) {
	vl, ok := val.GetValue().(link.MountedLink)
	if !ok {
		return
	}
	e.c.le.Debugf("lost link with uuid %v", vl.GetLinkUUID())
	tpl := pubsub.NewPeerLinkTuple(vl)
	e.c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		for i, l := range e.c.incLinks {
			if l == vl {
				e.c.incLinks[i] = e.c.incLinks[len(e.c.incLinks)-1]
				e.c.incLinks[len(e.c.incLinks)-1] = nil
				e.c.incLinks = e.c.incLinks[:len(e.c.incLinks)-1]
				break
			}
		}
		if v, ok := e.c.links[tpl]; ok {
			v.ctxCancel()
		}
	})
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (e *establishLinkHandler) HandleInstanceDisposed(inst directive.Instance) {
	eref := e.ref
	if eref == nil {
		return
	}
	e.ref = nil

	e.c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		for i, ref := range e.c.cleanupRefs {
			if ref == eref {
				a := e.c.cleanupRefs
				a[i] = a[len(a)-1]
				a[len(a)-1] = nil
				a = a[:len(a)-1]
				e.c.cleanupRefs = a
				break
			}
		}
	})
}

var _ directive.ReferenceHandler = ((*establishLinkHandler)(nil))
