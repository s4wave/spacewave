package link_holdopen_controller

import (
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// establishLinkHandler handles EstablishLink values
type establishLinkHandler struct {
	// c is the controller
	c *Controller
	// le is the logger
	le *logrus.Entry
	// ref is the reference
	ref directive.Reference
	// peerID is the peer id in the directive
	peerID peer.ID
	// di is the directive instance
	di directive.Instance

	// mtx guards below fields
	mtx sync.Mutex
	// valCount is the number of added values
	valCount int
	// rigidRef is the non-weak reference
	rigidRef directive.Reference
}

// newEstablishLinkHandler constructs a new establishLinkHandler
func newEstablishLinkHandler(
	c *Controller,
	le *logrus.Entry,
	di directive.Instance,
	peerID peer.ID,
) *establishLinkHandler {
	return &establishLinkHandler{
		c:      c,
		di:     di,
		le:     le.WithField("peer-id", peerID.String()),
		peerID: peerID,
	}
}

// HandleValueAdded is called when a value is added to the directive.
func (e *establishLinkHandler) HandleValueAdded(inst directive.Instance, val directive.AttachedValue) {
	vl, ok := val.GetValue().(link.MountedLink)
	if !ok || vl == nil {
		return
	}
	e.mtx.Lock()
	e.valCount++
	nrr := e.rigidRef == nil
	e.mtx.Unlock()

	if nrr {
		e.le.
			WithField("link-uuid", vl.GetLinkUUID()).
			WithField("local-peer", vl.GetLocalPeer().String()).
			Debug("starting peer hold-open tracking")
		go func() {
			e.mtx.Lock()
			e.rigidRef = e.di.AddReference(nil, false)
			e.mtx.Unlock()
		}()
	}
}

// HandleValueRemoved is called when a value is removed from the directive.
func (e *establishLinkHandler) HandleValueRemoved(inst directive.Instance, val directive.AttachedValue) {
	e.mtx.Lock()
	if e.valCount > 0 {
		e.valCount--
	}
	if e.valCount == 0 && e.rigidRef != nil {
		go e.rigidRef.Release()
		e.rigidRef = nil
	}
	e.mtx.Unlock()
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (e *establishLinkHandler) HandleInstanceDisposed(inst directive.Instance) {
	e.mtx.Lock()

	eref := e.ref
	if eref == nil {
		e.mtx.Unlock()
		return
	}
	e.ref = nil
	if e.rigidRef != nil {
		go e.rigidRef.Release()
		e.rigidRef = nil
	}
	e.mtx.Unlock()

	e.c.mtx.Lock()
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
	e.c.mtx.Unlock()
}

var _ directive.ReferenceHandler = ((*establishLinkHandler)(nil))
