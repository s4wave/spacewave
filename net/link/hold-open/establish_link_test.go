package link_holdopen_controller

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// holdInstance is a directive instance that blocks AddReference until
// released and counts the references that remain held.
type holdInstance struct {
	directive.Instance

	// added receives once per AddReference call.
	added chan struct{}
	// proceed unblocks one pending AddReference call.
	proceed chan struct{}
	// released receives once per released reference.
	released chan struct{}
	// held is the number of references added and not yet released.
	held atomic.Int32
}

// AddReference blocks until proceed and returns a counted reference.
func (h *holdInstance) AddReference(cb directive.ReferenceHandler, weakRef bool) directive.Reference {
	h.added <- struct{}{}
	<-h.proceed
	h.held.Add(1)
	return &holdReference{h: h}
}

// holdReference is a reference returned by holdInstance.
type holdReference struct {
	h *holdInstance
}

// Release decrements the held count and signals the release.
func (r *holdReference) Release() {
	r.h.held.Add(-1)
	r.h.released <- struct{}{}
}

// mountedLink is a mounted link value with fixed identifiers.
type mountedLink struct {
	link.MountedLink
}

// GetLinkUUID returns a fixed link id.
func (mountedLink) GetLinkUUID() uint64 { return 1 }

// GetLocalPeer returns an empty peer id.
func (mountedLink) GetLocalPeer() peer.ID { return "" }

// attachedValue wraps a mounted link as a directive value.
type attachedValue struct {
	directive.AttachedValue
}

// GetValue returns the mounted link.
func (attachedValue) GetValue() directive.Value { return mountedLink{} }

// TestHoldReleasedWhenValueRemovedBeforeAdd checks that removing the last
// link while the non-weak reference is still being added releases it.
func TestHoldReleasedWhenValueRemovedBeforeAdd(t *testing.T) {
	// Construct the handler over a directive that blocks AddReference.
	di := &holdInstance{
		added:    make(chan struct{}, 4),
		proceed:  make(chan struct{}),
		released: make(chan struct{}, 4),
	}
	e := newEstablishLinkHandler(nil, logrus.NewEntry(logrus.New()), di, "")

	// Add two links and remove both while the reference is still pending.
	e.HandleValueAdded(di, attachedValue{})
	e.HandleValueAdded(di, attachedValue{})
	<-di.added
	e.HandleValueRemoved(di, attachedValue{})
	e.HandleValueRemoved(di, attachedValue{})

	// Let the pending reference finish and expect it to be released.
	close(di.proceed)
	select {
	case <-di.released:
	case <-time.After(5 * time.Second):
		t.Fatal("pending reference was never released")
	}
	if n := di.held.Load(); n != 0 {
		t.Fatalf("expected no held references, got %d", n)
	}
}
