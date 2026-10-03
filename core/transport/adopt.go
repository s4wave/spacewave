package transport

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	net_transport "github.com/s4wave/spacewave/net/transport"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// AdoptLink takes ownership of an authenticated link for this Session. It
// registers ordinary stream routing and telemetry through the transport
// controller. Closing the link or this Session releases the controller.
// The link's local identity must already be authenticated as this Session.
func (t *SessionTransport) AdoptLink(ctx context.Context, lnk link.Link) error {
	// Require the attached link to authenticate as the session peer.
	if lnk.GetLocalPeer() != t.peerID {
		_ = lnk.Close()
		return errors.New("attached link belongs to another Session")
	}

	// Wait for the session transport to accept links.
	if err := t.AwaitReady(ctx); err != nil {
		_ = lnk.Close()
		return err
	}

	// Require a live session bus and controller lifetime for the attached link.
	var b bus.Bus
	var owner context.Context
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { b, owner = t.childBus, t.lifecycleCtx })
	if b == nil || owner == nil || owner.Err() != nil {
		_ = lnk.Close()
		return errors.New("Session transport has closed")
	}

	// Register an attached transport controller under the session lifetime.
	linkCtx, cancel := context.WithCancel(owner)
	attached := &attachedTransport{lnk: lnk, cancel: cancel}
	ctrl := transport_controller.NewController(t.le, b,
		controller.NewInfo(fmt.Sprintf("alpha/transport/attached/%d", lnk.GetUUID()), controller.MustParseVersion("0.0.1"), "authenticated attached connection"),
		t.peerID, false, func(_ context.Context, _ *logrus.Entry, _ crypto.PrivKey, handler net_transport.TransportHandler) (net_transport.Transport, error) {
			attached.handler = handler
			return attached, nil
		})
	release, err := b.AddController(linkCtx, ctrl, nil)
	if err != nil {
		_ = attached.Close()
		return err
	}

	// Wait for the attached transport controller to expose its link.
	if _, err := ctrl.GetTransport(ctx); err != nil {
		release()
		_ = attached.Close()
		return err
	}

	// Retain the attached controller while the same session bus is active.
	var retained bool
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.childBus == b && linkCtx.Err() == nil {
			t.linkControllers = append(t.linkControllers, ctrl)
			retained = true
			broadcast()
		}
	})
	if !retained {
		release()
		_ = attached.Close()
		return errors.New("Session transport closed while attaching the link")
	}

	// Release the attached controller when its session context ends.
	context.AfterFunc(linkCtx, func() {
		release()
		t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			t.linkControllers = slices.DeleteFunc(t.linkControllers, func(current *transport_controller.Controller) bool { return current == ctrl })
			broadcast()
		})
	})
	return nil
}

// attachedTransport delegates all stream and link lifetime rules to the common
// controller while retaining the already established connection.
type attachedTransport struct {
	lnk       link.Link
	handler   net_transport.TransportHandler
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func (t *attachedTransport) GetUUID() uint64    { return t.lnk.GetUUID() }
func (t *attachedTransport) GetPeerID() peer.ID { return t.lnk.GetLocalPeer() }

func (t *attachedTransport) Execute(ctx context.Context) error {
	// Expose the attached link until the controller context ends.
	lnk := &attachedLink{Link: t.lnk, transport: t}
	t.handler.HandleLinkEstablished(lnk)
	defer t.handler.HandleLinkLost(lnk)
	<-ctx.Done()
	return ctx.Err()
}

func (t *attachedTransport) Close() error {
	t.closeOnce.Do(func() { t.cancel(); _ = t.lnk.Close() })
	return nil
}

type attachedLink struct {
	link.Link
	transport *attachedTransport
}

func (l *attachedLink) Close() error { return l.transport.Close() }
