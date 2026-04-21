package rwc

import (
	"io"
	"net"
	"time"
)

// RwcPacketConn wraps a ReadWriteCloser to implement net.PacketConn.
//
// Read and write deadlines are not implemented.
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
func (pc *RwcPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = pc.rwc.Read(p)
	// Address is set to remote address for each read packet
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
// No-op: RwcPacketConn does not support deadlines
func (pc *RwcPacketConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline sets the deadline for future Read calls.
// No-op: RwcPacketConn does not support deadlines
func (pc *RwcPacketConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline sets the deadline for future Write calls.
// No-op: RwcPacketConn does not support deadlines
func (pc *RwcPacketConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// _ is a type assertion
var _ net.PacketConn = ((*RwcPacketConn)(nil))
