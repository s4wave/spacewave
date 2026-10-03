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
	// holding is set while the handler wants a non-weak reference.
	holding bool
	// rigidRef is the non-weak reference, nil until it is added.
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
	// Accept only mounted-link values from the directive.
	vl, ok := val.GetValue().(link.MountedLink)
	if !ok || vl == nil {
		return
	}

	// Count active values and claim the hold when none is wanted yet.
	e.mtx.Lock()
	e.valCount++
	start := !e.holding
	e.holding = true
	e.mtx.Unlock()
	if !start {
		return
	}

	// Add the non-weak reference outside the directive callback. A removal
	// or disposal that clears holding before it is stored releases it.
	e.le.
		WithField("link-uuid", vl.GetLinkUUID()).
		WithField("local-peer", vl.GetLocalPeer().String()).
		Debug("starting peer hold-open tracking")
	go func() {
		// Store the reference unless the hold ended or another one won.
		ref := e.di.AddReference(nil, false)
		e.mtx.Lock()
		keep := e.holding && e.rigidRef == nil
		if keep {
			e.rigidRef = ref
		}
		e.mtx.Unlock()

		// Release a reference that is no longer wanted.
		if !keep {
			ref.Release()
		}
	}()
}

// releaseHold stops holding the directive and releases the non-weak reference
// if it was stored. Called with mtx held.
func (e *establishLinkHandler) releaseHold() {
	e.holding = false
	if e.rigidRef != nil {
		go e.rigidRef.Release()
		e.rigidRef = nil
	}
}

// HandleValueRemoved is called when a value is removed from the directive.
func (e *establishLinkHandler) HandleValueRemoved(inst directive.Instance, val directive.AttachedValue) {
	// Update the value count under the handler lock.
	e.mtx.Lock()
	if e.valCount > 0 {
		e.valCount--
	}

	// Stop holding the directive when the final value is removed.
	if e.valCount == 0 {
		e.releaseHold()
	}
	e.mtx.Unlock()
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (e *establishLinkHandler) HandleInstanceDisposed(inst directive.Instance) {
	// Detach the controller reference and any rigid value reference.
	e.mtx.Lock()
	eref := e.ref
	if eref == nil {
		e.mtx.Unlock()
		return
	}
	e.ref = nil
	e.releaseHold()
	e.mtx.Unlock()

	// Remove the disposed reference from controller cleanup state.
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

// _ is a type assertion
var _ directive.ReferenceHandler = (*establishLinkHandler)(nil)
