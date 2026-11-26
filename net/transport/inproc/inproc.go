package inproc

import (
	"context"
	"net"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/common/pconn"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// TransportType is the transport type string for dial addresses.
const TransportType = "inproc"

// ControllerID is the controller identifier.
const ControllerID = "bifrost/inproc"

// Version is the version of the inproc implementation.
var Version = controller.MustParseVersion("0.0.1")

// Inproc carries authenticated peer links over connected in-process packet endpoints.
type Inproc struct {
	// Transport owns packet encryption and authenticated peer links.
	*pconn.Transport

	// le records transport activity.
	le *logrus.Entry
	// packetConn receives packets for this endpoint.
	packetConn *packetConn
	// localAddr identifies this endpoint.
	localAddr net.Addr

	// bcast guards remote endpoints and wakes pending dialers.
	bcast broadcast.Broadcast
	// remotes maps connected addresses to their receiving endpoints.
	remotes map[string]*Inproc
}

// NewInproc builds a new Inproc transport.
// Yields Links to other Inproc transports.
func NewInproc(
	ctx context.Context,
	le *logrus.Entry,
	opts *Config,
	pKey crypto.PrivKey,
	c transport.TransportHandler,
) (transport.Transport, error) {
	// Derive the local peer identity for this in-process transport.
	peerID, err := peer.IDFromPrivateKey(pKey)
	if err != nil {
		return nil, err
	}

	// Allocate the endpoint that the packet writer will call.
	localAddr := NewAddr(peerID)
	ip := new(Inproc)

	// Create the packet connection that routes to known remotes.
	npc := newPacketConn(
		ctx,
		localAddr,
		ip.writeToAddr,
	)

	// Construct the packet-backed transport.
	tpt, err := pconn.NewTransport(
		ctx,
		le,
		pKey,
		c,
		opts.GetPacketOpts(),
		0,
		npc,
		ParseAddr,
		opts.GetDialers(),
	)
	if err != nil {
		return nil, err
	}

	// Complete the endpoint before returning it to the transport controller.
	*ip = Inproc{
		Transport:  tpt,
		le:         le,
		packetConn: npc,
		localAddr:  localAddr,
		remotes:    make(map[string]*Inproc),
	}
	return ip, nil
}

// BuildInprocController constructs the in-proc transport controller.
func BuildInprocController(
	le *logrus.Entry,
	b bus.Bus,
	peerIDConstraint peer.ID,
	conf *Config,
) *transport_controller.Controller {
	return transport_controller.NewController(
		le,
		b,
		controller.NewInfo(ControllerID, Version, "in-proc transport"),
		peerIDConstraint,
		conf.GetVerbose(),
		func(
			ctx context.Context,
			le *logrus.Entry,
			pkey crypto.PrivKey,
			handler transport.TransportHandler,
		) (transport.Transport, error) {
			return NewInproc(
				ctx,
				le,
				conf,
				pkey,
				handler,
			)
		},
	)
}

// MatchTransportType checks if the given transport type ID matches this transport.
// If returns true, the transport controller will call DialPeer with that tptaddr.
// Examples include "udp-quic" and "ws".
func (t *Inproc) MatchTransportType(transportType string) bool {
	return transportType == TransportType
}

// ConnectToInproc connects the inproc to a remote inproc.
// It replaces the existing route for the same peer.
func (t *Inproc) ConnectToInproc(ctx context.Context, other *Inproc) {
	// Record the remote packet endpoint under its address.
	oa := other.localAddr.String()
	t.bcast.HoldLock(func(notify func(), _ func() <-chan struct{}) {
		t.remotes[oa] = other
		notify()
	})
}

// DisconnectInproc disconnects a previously connected inproc.
func (t *Inproc) DisconnectInproc(ctx context.Context, other *Inproc) {
	// Remove the remote packet endpoint from the routing table.
	oa := other.localAddr.String()
	t.bcast.HoldLock(func(notify func(), _ func() <-chan struct{}) {
		if t.remotes[oa] == other {
			delete(t.remotes, oa)
			notify()
		}
	})
}

// GetPeerDialer waits for an attached peer unless an explicit dialer is configured.
// Connection changes wake standing link requests without a polling loop.
func (t *Inproc) GetPeerDialer(ctx context.Context, peerID peer.ID) (*dialer.DialerOpts, error) {
	// Preserve explicitly configured dialing options.
	if opts, err := t.Transport.GetPeerDialer(ctx, peerID); err != nil || opts != nil {
		return opts, err
	}
	address := NewAddr(peerID).String()
	for {
		var remote *Inproc
		var waitCh <-chan struct{}
		t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			remote = t.remotes[address]
			waitCh = getWaitCh()
		})
		if remote != nil {
			return &dialer.DialerOpts{Address: address}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
	}
}

// DialPeer chooses one initiator for both directions of an in-process peer pair.
// Concurrent callers share that transport's existing address dialer, so they cannot
// replace each other's authenticated connection while streams are opening.
func (t *Inproc) DialPeer(ctx context.Context, peerID peer.ID, address string) (link.Link, bool, error) {
	// Wait for the endpoint addressed by the caller's standing link request.
	var remote *Inproc
	for remote == nil {
		var waitCh <-chan struct{}
		t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			remote = t.remotes[address]
			waitCh = getWaitCh()
		})
		if remote != nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-waitCh:
		}
	}
	if peerID != "" && remote.GetPeerID() != peerID {
		return nil, true, errors.New("in-process address does not match requested peer")
	}

	// Both sides delegate to the lower peer's authenticated QUIC dialer.
	if t.GetPeerID() < remote.GetPeerID() {
		return t.Transport.DialPeer(ctx, remote.GetPeerID(), address)
	}
	_, fatal, err := remote.Transport.DialPeer(ctx, t.GetPeerID(), t.localAddr.String())
	return nil, fatal, err
}

// writeToAddr routes outgoing packets.
func (t *Inproc) writeToAddr(ctx context.Context, p []byte, addr net.Addr) (int, error) {
	// Resolve the remote endpoint while holding the routing mutex.
	oa := addr.String()
	var out *Inproc
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { out = t.remotes[oa] })

	// Reject packets for an endpoint that is not connected.
	if out == nil {
		return 0, &net.AddrError{
			Addr: oa,
			Err:  "remote transport not connected",
		}
	}

	// Deliver the packet to the remote in-process connection.
	return out.packetConn.HandlePacket(ctx, p, t.localAddr)
}

// _ is a type assertion.
var _ transport.Transport = (*Inproc)(nil)

// _ is a type assertion.
var _ dialer.TransportDialer = (*Inproc)(nil)
