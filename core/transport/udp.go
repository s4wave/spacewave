//go:build !tinygo

package transport

import (
	"context"
	"maps"
	"net"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	transport_udp "github.com/s4wave/spacewave/net/transport/udp"
)

// UDPConfig configures one UDP transport run under the Session's peer.
type UDPConfig struct {
	// ListenAddr is the host:port to listen on, or an ephemeral port when empty.
	ListenAddr string
	// Peers names the remote peers whose links mount, each with the host:port
	// it listens on. A peer with an empty address is admitted when it dials
	// in but is never dialed.
	Peers map[peer.ID]string
	// NamedOnly mounts only the links of named peers. Otherwise a link from an
	// unnamed peer mounts when it passes AuthorizeLink.
	NamedOnly bool
}

// StartUDP runs a UDP transport under the Session's peer until ctx ends or the
// current transport run stops. It dials each named peer that has an address
// and holds its link while the transport runs.
// Naming a peer, or the link authorizer admitting it, admits only its link:
// services that grant authority, such as those served with ServeAuthorized,
// still apply AuthorizePeer to each stream.
// StartUDP returns the bound local address once the socket is open.
func (t *SessionTransport) StartUDP(ctx context.Context, conf UDPConfig) (net.Addr, error) {
	// Capture the running child bus and the lifetime that owns it.
	var b bus.Bus
	var lifecycleCtx context.Context
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		b, lifecycleCtx = t.childBus, t.lifecycleCtx
	})
	if b == nil {
		return nil, errors.New("session transport is not running")
	}

	// Build the gated controller with a static dialer for each named peer
	// that has an address, admitting the named peers' links.
	peers := maps.Clone(conf.Peers)
	dialers := make(map[string]*dialer.DialerOpts, len(peers))
	for id, addr := range peers {
		if addr != "" {
			dialers[id.String()] = &dialer.DialerOpts{Address: addr}
		}
	}
	authorize := func(ctx context.Context, remotePeer peer.ID) error {
		if _, ok := peers[remotePeer]; ok {
			return nil
		}
		if conf.NamedOnly {
			return errors.Errorf("peer %s is not named on this udp transport", remotePeer)
		}
		return t.AuthorizeLink(ctx, remotePeer)
	}
	ctrl, err := transport_udp.NewController(t.le, b, &transport_udp.Config{
		TransportPeerId: t.peerID.String(),
		ListenAddr:      conf.ListenAddr,
		Dialers:         dialers,
	}, transport_controller.WithPeerAuthorizer(authorize))
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
		if err := holdDialedLinks(runCtx, b, t.peerID, dialers); err != nil {
			cancel()
			return nil, err
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

// holdDialedLinks holds a link to each peer in dialers until ctx ends. A static
// dialer answers only a link request, and a link nobody holds closes after its
// hold-open, so without the hold a named address would never be dialed.
func holdDialedLinks(ctx context.Context, b bus.Bus, local peer.ID, dialers map[string]*dialer.DialerOpts) error {
	for id := range dialers {
		remote, err := peer.IDB58Decode(id)
		if err != nil {
			return err
		}
		hold := directive.NewTypedCallbackHandler[link.MountedLink](
			func(directive.TypedAttachedValue[link.MountedLink]) {}, nil, nil, nil,
		)
		_, ref, err := b.AddDirective(ctx, link.NewEstablishLinkWithPeer(local, remote), hold)
		if err != nil {
			return err
		}
		context.AfterFunc(ctx, ref.Release)
	}
	return nil
}
