package stream_api_rpc

import (
	"io"
	"net"
	"time"

	"github.com/s4wave/spacewave/net/peer"
)

// NetConn wraps an RPC into a net.Conn compat interface.
type NetConn struct {
	rpc          RPC
	localPeerID  peer.ID
	remotePeerID peer.ID
}

// NewNetConn constructs a new NetConn.
func NewNetConn(localPeerID, remotePeerID peer.ID, rpc RPC) *NetConn {
	return &NetConn{
		rpc:          rpc,
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
	}
}

// Read reads data from the connection.
// Read can be made to time out and return an Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (n *NetConn) Read(b []byte) (int, error) {
	for {
		data, err := n.rpc.Recv()
		if err != nil {
			return 0, err
		}

		buf := data.Data
		if len(buf) == 0 {
			continue
		}

		copy(b, buf)
		if len(buf) > len(b) {
			return len(buf), io.ErrShortBuffer
		}

		return len(buf), nil
	}
}

// Write writes data to the connection.
// Write can be made to time out and return an Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (n *NetConn) Write(b []byte) (nw int, err error) {
	nw = len(b)
	err = n.rpc.Send(&Data{
		Data: b,
	})
	return
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (n *NetConn) Close() error {
	// TODO
	return nil
}

// LocalAddr returns the local network address.
func (n *NetConn) LocalAddr() net.Addr {
	if n.localPeerID == "" {
		return &net.UDPAddr{IP: net.IP{127, 0, 0, 1}, Port: 2}
	}

	return peer.NewNetAddr(n.localPeerID)
}

// RemoteAddr returns the remote network address.
func (n *NetConn) RemoteAddr() net.Addr {
	if n.remotePeerID == "" {
		return &net.UDPAddr{IP: net.IP{127, 0, 0, 1}, Port: 3}
	}

	return peer.NewNetAddr(n.remotePeerID)
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A zero value for t means I/O operations will not time out.
// Deadlines are stubbed here.
func (n *NetConn) SetDeadline(t time.Time) error {
	_ = n.SetReadDeadline(t)
	_ = n.SetWriteDeadline(t)
	return nil
}

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (n *NetConn) SetReadDeadline(t time.Time) error {
	// TODO: stub
	return nil
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (n *NetConn) SetWriteDeadline(t time.Time) error {
	// TODO: stub
	return nil
}

// _ is a type assertion
var _ net.Conn = ((*NetConn)(nil))
