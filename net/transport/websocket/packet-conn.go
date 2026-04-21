package websocket

import (
	"context"
	"io"
	"net"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
)

// PacketConn implements the PacketConn interface with a websocket.
type PacketConn struct {
	ctx   context.Context
	ws    *websocket.Conn
	laddr net.Addr
	raddr net.Addr

	rd time.Time // read deadline
	wd time.Time // write deadline
}

// NewPacketConn constructs a new packet conn from a WebSocket conn.
func NewPacketConn(ctx context.Context, ws *websocket.Conn, laddr, raddr net.Addr) *PacketConn {
	return &PacketConn{ctx: ctx, ws: ws, laddr: laddr, raddr: raddr}
}

// LocalAddr returns the local network address.
func (c *PacketConn) LocalAddr() net.Addr {
	return c.laddr
}

// ReadMessage reads a single message from the connection.
func (c *PacketConn) ReadMessage() ([]byte, error) {
	ctx := c.ctx
	if rd := c.rd; !rd.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, rd)
		defer cancel()
	}

	_, data, err := c.ws.Read(ctx)
	return data, err
}

// ReadFrom reads a packet from the connection.
func (c *PacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	select {
	case <-c.ctx.Done():
		return 0, nil, io.EOF
	default:
	}

	data, err := c.ReadMessage()
	if err != nil {
		// Detect EOF wrapped with "failed to get reader
		if err == context.Canceled || errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
			err = io.EOF
		}
		return 0, nil, err
	}
	dlen := len(data)
	copy(p, data)
	if len(p) < len(data) {
		err = io.ErrShortBuffer
	}
	return dlen, c.raddr, err
}

// WriteMessage writes a single message to the connection.
func (c *PacketConn) WriteMessage(msg []byte) error {
	ctx := c.ctx
	if wd := c.wd; !wd.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, wd)
		defer cancel()
	}
	return c.ws.Write(ctx, websocket.MessageBinary, msg)
}

// WriteTo writes a packet with payload p to addr.
// WriteTo can be made to time out and return an Error after a
// fixed time limit; see SetDeadline and SetWriteDeadline.
// On packet-oriented connections, write timeouts are rare.
func (c *PacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if addr != c.raddr {
		as := addr.String()
		rs := c.raddr.String()
		if as != rs {
			return 0, errors.Errorf("packet conn bound to %s and cannot write to %s", rs, as)
		}
	}
	if err := c.WriteMessage(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail instead of blocking. The deadline applies to all future
// and pending I/O, not just the immediately following call to
// Read or Write. After a deadline has been exceeded, the
// connection can be refreshed by setting a deadline in the future.
//
// If the deadline is exceeded a call to Read or Write or to other
// I/O methods will return an error that wraps os.ErrDeadlineExceeded.
// This can be tested using errors.Is(err, os.ErrDeadlineExceeded).
// The error's Timeout method will return true, but note that there
// are other possible errors for which the Timeout method will
// return true even if the deadline has not been exceeded.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful ReadFrom or WriteTo calls.
//
// A zero value for t means I/O operations will not time out.
func (c *PacketConn) SetDeadline(t time.Time) error {
	c.rd = t
	c.wd = t
	return nil
}

// SetReadDeadline sets the deadline for future ReadFrom calls
// and any currently-blocked ReadFrom call.
// A zero value for t means ReadFrom will not time out.
func (c *PacketConn) SetReadDeadline(t time.Time) error {
	c.rd = t
	return nil
}

// SetWriteDeadline sets the deadline for future WriteTo calls
// and any currently-blocked WriteTo call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means WriteTo will not time out.
func (c *PacketConn) SetWriteDeadline(t time.Time) error {
	c.wd = t
	return nil
}

// Close closes the connection.
func (c *PacketConn) Close() error {
	_ = c.ws.Close(websocket.StatusGoingAway, "goodbye")
	return nil
}

// _ is a type assertion
var _ net.PacketConn = ((*PacketConn)(nil))
