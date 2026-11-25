package rwc

import (
	"io"
	"net"
	"time"

	"github.com/pkg/errors"
)

// RwcPacketConn wraps a ReadWriteCloser to implement net.PacketConn.
//
// Deadlines pass to the ReadWriteCloser when it has them, as a WebRTC data
// channel does, and are no-ops otherwise. A deadline must reach the stream: a
// closed data channel only half-closes, so a read blocked on it waits until
// the remote peer resets the stream, and quic-go's Transport.Close wakes its
// read loop with SetReadDeadline and waits for that loop to return. Without
// the deadline a handshake that timed out over a failed path never returns.
// The addresses are ignored when reading / writing.
type RwcPacketConn struct {
	rwc   io.ReadWriteCloser
	laddr net.Addr
	raddr net.Addr
}

// NewRwcPacketConn creates a new RwcPacketConn with the provided ReadWriteCloser and local/remote addresses.
func NewRwcPacketConn(rwc io.ReadWriteCloser, laddr, raddr net.Addr) *RwcPacketConn {
	return &RwcPacketConn{rwc: rwc, laddr: laddr, raddr: raddr}
}

// ReadFrom reads a packet from the connection, ignoring the provided address.
//
// Read errors are wrapped so they are not a net.Error, and quic-go ends its
// read loop on one instead of retrying: a deadline is set only to stop the
// loop, and no other read error clears by retrying. errors.Is still finds the
// cause, such as io.EOF.
func (pc *RwcPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = pc.rwc.Read(p)
	if err != nil {
		err = errors.Wrap(err, "read packet")
	}
	return n, pc.raddr, err
}

// WriteTo writes a packet to the connection, ignoring the provided address.
func (pc *RwcPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return pc.rwc.Write(p)
}

// Close closes the connection.
func (pc *RwcPacketConn) Close() error {
	return pc.rwc.Close()
}

// LocalAddr returns the local network address.
func (pc *RwcPacketConn) LocalAddr() net.Addr {
	return pc.laddr
}

// SetDeadline sets the read and write deadlines associated with the connection.
func (pc *RwcPacketConn) SetDeadline(t time.Time) error {
	if err := pc.SetReadDeadline(t); err != nil {
		return err
	}
	return pc.SetWriteDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls and any blocked one.
// It is a no-op when the ReadWriteCloser has no read deadline.
func (pc *RwcPacketConn) SetReadDeadline(t time.Time) error {
	if d, ok := pc.rwc.(interface{ SetReadDeadline(time.Time) error }); ok {
		return d.SetReadDeadline(t)
	}
	return nil
}

// SetWriteDeadline sets the deadline for future Write calls.
// It is a no-op when the ReadWriteCloser has no write deadline.
func (pc *RwcPacketConn) SetWriteDeadline(t time.Time) error {
	if d, ok := pc.rwc.(interface{ SetWriteDeadline(time.Time) error }); ok {
		return d.SetWriteDeadline(t)
	}
	return nil
}

// _ is a type assertion
var _ net.PacketConn = (*RwcPacketConn)(nil)
