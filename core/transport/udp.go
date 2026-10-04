//go:build !tinygo

package transport

import (
	"context"
	"net"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	transport_udp "github.com/s4wave/spacewave/net/transport/udp"
)

// StartUDP runs a UDP transport under the Session's peer until ctx ends or the
// current transport run stops. It listens on listenAddr, or an ephemeral port
// when empty, and dials each peer in peerAddrs at its host:port address. Every
// link, inbound or dialed, must pass AuthorizePeer before it mounts. StartUDP
// returns the bound local address once the socket is open.
func (t *SessionTransport) StartUDP(ctx context.Context, listenAddr string, peerAddrs map[peer.ID]string) (net.Addr, error) {
	// Capture the running child bus and the lifetime that owns it.
	var b bus.Bus
	var lifecycleCtx context.Context
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		b, lifecycleCtx = t.childBus, t.lifecycleCtx
	})
	if b == nil {
		return nil, errors.New("session transport is not running")
	}

	// Build the gated controller with a static dialer for each named peer.
	dialers := make(map[string]*dialer.DialerOpts, len(peerAddrs))
	for id, addr := range peerAddrs {
		dialers[id.String()] = &dialer.DialerOpts{Address: addr}
	}
	ctrl, err := transport_udp.NewController(t.le, b, &transport_udp.Config{
		TransportPeerId: t.peerID.String(),
		ListenAddr:      listenAddr,
		Dialers:         dialers,
	}, transport_controller.WithPeerAuthorizer(t.AuthorizePeer))
	if err != nil {
		return nil, err
	}

	// Run the controller for the shorter of ctx and the transport run, and
	// expose its links with the Session's other transports meanwhile.
	runCtx, cancel := context.WithCancel(ctx)
	stopWithRun := context.AfterFunc(lifecycleCtx, cancel)
	t.addLinkController(ctrl)
	exited := make(chan error, 1)
	go func() {
		// Withdraw the controller's links once it exits.
		defer t.removeLinkController(ctrl)
		defer stopWithRun()
		defer cancel()

		// Log an unexpected exit and report it to the bind wait.
		err := b.ExecuteController(runCtx, ctrl)
		if err != nil && runCtx.Err() == nil {
			t.le.WithError(err).Warn("udp transport stopped")
		}
		exited <- err
	}()

	// Wait for the bound socket or the controller's failure.
	bound := make(chan transport.Transport, 1)
	go func() {
		if tpt, err := ctrl.GetTransport(runCtx); err == nil {
			bound <- tpt
		}
	}()
	select {
	case tpt := <-bound:
		addr, ok := tpt.(interface{ LocalAddr() net.Addr })
		if !ok {
			cancel()
			return nil, errors.New("udp transport has no local address")
		}
		return addr.LocalAddr(), nil
	case err := <-exited:
		if err == nil {
			err = errors.New("udp transport exited")
		}
		return nil, errors.Wrap(err, "start udp transport")
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}
