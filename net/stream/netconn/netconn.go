package stream_netconn

import (
	"net"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
)

// NetConn wraps a Bifrost stream to be compatible with net.Conn.
type NetConn struct {
	stream.Stream

	localPeerID  peer.ID
	remotePeerID peer.ID
}

// NewNetConn constructs a net.Conn from a stream.
func NewNetConn(strm link.MountedStream) net.Conn {
	return &NetConn{
		Stream:       strm.GetStream(),
		remotePeerID: strm.GetPeerID(),
		localPeerID:  strm.GetLink().GetLocalPeer(),
	}
}

// LocalAddr returns the local network address.
func (n *NetConn) LocalAddr() net.Addr {
	return peer.NewNetAddr(n.localPeerID)
}

// RemoteAddr returns the remote network address.
func (n *NetConn) RemoteAddr() net.Addr {
	return peer.NewNetAddr(n.remotePeerID)
}

// _ is a type assertion
var _ net.Conn = ((*NetConn)(nil))
