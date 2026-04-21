package udp

import (
	"context"
	"net"

	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/common/pconn"
	"github.com/sirupsen/logrus"
)

// TransportType is the transport type identifier.
const TransportType = "udp"

// ControllerID is the UDP controller ID.
const ControllerID = "bifrost/udp"

// Version is the version of the udp implementation.
var Version = semver.MustParse("0.0.1")

// ExtendedSockBuf is the sockbuf parameter to set on udp sockets.
var ExtendedSockBuf = 16777217

// UDP implements a UDP transport.
type UDP struct {
	*pconn.Transport
}

// NewUDP builds a new UDP transport, listening on the addr.
//
// uuid can be empty to generate based on peer id and local addr
func NewUDP(
	ctx context.Context,
	le *logrus.Entry,
	pKey crypto.PrivKey,
	c transport.TransportHandler,
	pconnOpts *pconn.Opts,
	uuid uint64,
	listenAddr string,
	staticPeerMap map[string]*dialer.DialerOpts,
) (*UDP, error) {
	pc, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		return nil, err
	}
	if uc, ok := pc.(*net.UDPConn); ok {
		if err := uc.SetReadBuffer(ExtendedSockBuf); err != nil {
			le.WithError(err).Warn("unable to set read buffer on conn")
		}
		if err := uc.SetWriteBuffer(ExtendedSockBuf); err != nil {
			le.WithError(err).Warn("unable to set write buffer on conn")
		}
	}

	pct, err := pconn.NewTransport(
		ctx,
		le,
		pKey,
		c,
		pconnOpts,
		uuid,
		pc,
		func(addr string) (net.Addr, error) {
			return net.ResolveUDPAddr("udp", addr)
		},
		staticPeerMap,
	)
	if err != nil {
		return nil, err
	}
	return &UDP{
		Transport: pct,
	}, nil
}

// MatchTransportType checks if the given transport type ID matches this transport.
// If returns true, the transport controller will call DialPeer with that tptaddr.
// E.x.: "udp-quic" or "ws"
func (u *UDP) MatchTransportType(transportType string) bool {
	return TransportType == transportType
}

var (
	// _ is a type assertion.
	_ transport.Transport = ((*UDP)(nil))

	// _ is a type assertion
	_ dialer.TransportDialer = ((*UDP)(nil))
)
