package inproc

import (
	"context"
	"io"
	"net"
	"time"
)

// packetConn implements net.PacketConn
type packetConn struct {
	// ctx is the context
	ctx context.Context
	// ctxCancel is the context canceler
	ctxCancel context.CancelFunc
	// localAddr is the local address
	localAddr net.Addr

	rd time.Time // read deadline
	wd time.Time // write deadline

	// packetCh is the packet channel
	packetCh chan *incPacket
	// writer is the writer
	// if the remote is unknown will return AddrError
	writer func(ctx context.Context, p []byte, addr net.Addr) (int, error)
}

// newPacketConn constructs a new packet conn
func newPacketConn(
	rctx context.Context,
	localAddr net.Addr,
	writer func(ctx context.Context, p []byte, addr net.Addr) (int, error),
) *packetConn {
	ctx, ctxCancel := context.WithCancel(rctx)
	return &packetConn{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		localAddr: localAddr,
		writer:    writer,

		packetCh: make(chan *incPacket, 10),
	}
}

// incPacket is an incoming packet
type incPacket struct {
	addr net.Addr
	data []byte
}

// ReadFrom reads a packet from the connection,
// copying the payload into p. It returns the number of
// bytes copied into p and the return address that
// was on the packet.
// It returns the number of bytes read (0 <= n <= len(p))
// and any error encountered. Callers should always process
// the n > 0 bytes returned before considering the error err.
// ReadFrom can be made to time out and return
// an Error with Timeout() == true after a fixed time limit;
// see SetDeadline and SetReadDeadline.
func (c *packetConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	deadline := c.rd
	ctx := c.ctx
	if !deadline.IsZero() {
		var ctxCancel context.CancelFunc
		ctx, ctxCancel = context.WithDeadline(c.ctx, deadline)
		defer ctxCancel()
	}

	var pkt *incPacket
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case pkt = <-c.packetCh:
	}

	copy(p, pkt.data)
	if len(p) < len(pkt.data) {
		return len(p), pkt.addr, io.ErrShortBuffer
	}
	return len(pkt.data), pkt.addr, nil
}

// HandlePacket intakes a packet, returning it from ReadFrom.
func (c *packetConn) HandlePacket(ctx context.Context, p []byte, addr net.Addr) (int, error) {
	data := make([]byte, len(p))
	copy(data, p)
	pkt := &incPacket{data: data, addr: addr}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case c.packetCh <- pkt:
		return len(data), nil
	}
}

// WriteTo writes a packet with payload p to addr.
// WriteTo can be made to time out and return
// an Error with Timeout() == true after a fixed time limit;
// see SetDeadline and SetWriteDeadline.
// On packet-oriented connections, write timeouts are rare.
func (c *packetConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	deadline := c.wd
	ctx := c.ctx
	if !deadline.IsZero() {
		var ctxCancel context.CancelFunc
		ctx, ctxCancel = context.WithDeadline(c.ctx, deadline)
		defer ctxCancel()
	}

	return c.writer(ctx, p, addr)
}

// LocalAddr returns the local network address.
func (c *packetConn) LocalAddr() net.Addr {
	return c.localAddr
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail with a timeout (see type Error) instead of
// blocking. The deadline applies to all future and pending
// I/O, not just the immediately following call to ReadFrom or
// WriteTo. After a deadline has been exceeded, the connection
// can be refreshed by setting a deadline in the future.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful ReadFrom or WriteTo calls.
//
// A zero value for t means I/O operations will not time out.
func (c *packetConn) SetDeadline(t time.Time) error {
	_ = c.SetReadDeadline(t)
	_ = c.SetWriteDeadline(t)
	return nil
}

// SetReadDeadline sets the deadline for future ReadFrom calls
// and any currently-blocked ReadFrom call.
// A zero value for t means ReadFrom will not time out.
func (c *packetConn) SetReadDeadline(t time.Time) error {
	c.rd = t
	return nil
}

// SetWriteDeadline sets the deadline for future WriteTo calls
// and any currently-blocked WriteTo call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means WriteTo will not time out.
func (c *packetConn) SetWriteDeadline(t time.Time) error {
	c.wd = t
	return nil
}

// Close closes the connection.
// Any blocked ReadFrom or WriteTo operations will be unblocked and return errors.
func (c *packetConn) Close() error {
	c.ctxCancel()
	return nil
}

// _ is a type assertion
var _ net.PacketConn = ((*packetConn)(nil))
