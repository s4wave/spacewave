package inproc

import (
	"context"
	"net"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/crypto"
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
var Version = semver.MustParse("0.0.1")

// Inproc implements a Inproc transport.
type Inproc struct {
	// Transport is the packet transport
	*pconn.Transport

	// le is the logger
	le *logrus.Entry
	// packetConn is the packet conn
	packetConn *packetConn
	// localAddr is the local addr
	localAddr net.Addr

	// mtx guards below
	mtx sync.Mutex
	// remotes are the currently known remotes
	// map is from string (net.addr.String()) to *packetConn
	remotes map[string]*packetConn
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
	peerID, err := peer.IDFromPrivateKey(pKey)
	if err != nil {
		return nil, err
	}

	localAddr := NewAddr(peerID)
	ip := &Inproc{
		le:        le,
		localAddr: localAddr,
		remotes:   make(map[string]*packetConn),
	}
	npc := newPacketConn(
		ctx,
		localAddr,
		ip.writeToAddr,
	)
	ip.Transport, err = pconn.NewTransport(
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
	ip.packetConn = npc
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
// E.x.: "udp-quic" or "ws"
func (t *Inproc) MatchTransportType(transportType string) bool {
	return transportType == TransportType
}

// ConnectToInproc connects the inproc to a remote inproc.
// Will overwrite any existing connection
func (t *Inproc) ConnectToInproc(ctx context.Context, other *Inproc) {
	oa := other.localAddr.String()
	t.mtx.Lock()
	t.remotes[oa] = other.packetConn
	t.mtx.Unlock()
}

// DisconnectInproc disconnects a previously connected inproc.
func (t *Inproc) DisconnectInproc(ctx context.Context, other *Inproc) {
	oa := other.localAddr.String()
	t.mtx.Lock()
	delete(t.remotes, oa)
	t.mtx.Unlock()
}

// writeToAddr routes outgoing packets.
func (t *Inproc) writeToAddr(ctx context.Context, p []byte, addr net.Addr) (int, error) {
	oa := addr.String()
	t.mtx.Lock()
	out, outOk := t.remotes[oa]
	t.mtx.Unlock()
	if !outOk {
		return 0, &net.AddrError{
			Addr: oa,
			Err:  "remote transport not connected",
		}
	}
	return out.HandlePacket(ctx, p, t.localAddr)
}

// _ is a type assertion.
var _ transport.Transport = ((*Inproc)(nil))

// _ is a type assertion.
var _ dialer.TransportDialer = ((*Inproc)(nil))
