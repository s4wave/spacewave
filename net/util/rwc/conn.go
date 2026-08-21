package rwc

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// connPktSize is the size of the buffers to use for packets for the Conn.
const connPktSize = 2048

// Conn implements a Conn with a buffered ReadWriteCloser.
type Conn struct {
	// ctx is the context
	ctx context.Context
	// ctxCancel is the context canceler
	ctxCancel context.CancelFunc
	// rwc is the read-write-closer
	rwc io.ReadWriteCloser
	// laddr is the local addr
	laddr net.Addr
	// raddr is the remote addr
	raddr net.Addr

	ar       sync.Pool   // packet arena
	rd       time.Time   // read deadline
	packetCh chan []byte // packet ch
	closeErr error
}

// NewConn constructs a new packet conn and starts the rx pump.
func NewConn(
	ctx context.Context,
	rwc io.ReadWriteCloser,
	laddr, raddr net.Addr,
	bufferPacketN int,
) *Conn {
	ctx, ctxCancel := context.WithCancel(ctx) //nolint:gosec // cancel stored on Conn and called by Close
	if bufferPacketN <= 0 {
		bufferPacketN = 10
	}

	c := &Conn{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		rwc:       rwc,
		laddr:     laddr,
		raddr:     raddr,
		packetCh:  make(chan []byte, bufferPacketN),
	}
	go func() {
		_ = c.rxPump()
	}()
	return c
}

// LocalAddr returns the local network address.
func (p *Conn) LocalAddr() net.Addr {
	return p.laddr
}

// RemoteAddr returns the bound remote network address.
func (p *Conn) RemoteAddr() net.Addr {
	return p.raddr
}

// Read reads data from the connection.
// Read can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetReadDeadline.
func (p *Conn) Read(b []byte) (n int, err error) {
	// Apply the configured read deadline to the connection context.
	deadline := p.rd
	ctx := p.ctx
	if !deadline.IsZero() {
		var ctxCancel context.CancelFunc
		ctx, ctxCancel = context.WithDeadline(ctx, deadline)
		defer ctxCancel()
	}

	// Wait for the next buffered payload or a connection close.
	var pkt []byte
	var ok bool
	select {
	case <-ctx.Done():
		if !deadline.IsZero() {
			return 0, os.ErrDeadlineExceeded
		}
		return 0, context.Canceled
	case pkt, ok = <-p.packetCh:
		if !ok {
			err = p.closeErr
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
	}

	// Copy the payload into the caller's buffer and release its arena slot.
	pl := len(pkt)
	copy(b, pkt)
	p.ar.Put(&pkt)
	if len(b) < pl {
		return len(b), io.ErrShortBuffer
	}
	return pl, nil
}

// Write writes data to the connection.
func (p *Conn) Write(pkt []byte) (n int, err error) {
	// Write the complete payload, retrying partial writes.
	if len(pkt) == 0 {
		return 0, nil
	}

	// Continue writing until the entire payload is transmitted.
	written := 0
	for written < len(pkt) {
		n, err = p.rwc.Write(pkt[written:])
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// SetDeadline sets the read deadline associated with the connection.
// It is equivalent to calling SetReadDeadline.
//
// Write deadlines are not supported; see SetWriteDeadline.
//
// A zero value for t means ReadFrom will not time out.
func (p *Conn) SetDeadline(t time.Time) error {
	p.rd = t
	return nil
}

// SetReadDeadline sets the deadline for future ReadFrom calls
// and any currently-blocked ReadFrom call.
// A zero value for t means ReadFrom will not time out.
func (p *Conn) SetReadDeadline(t time.Time) error {
	p.rd = t
	return nil
}

// SetWriteDeadline is a no-op: Conn does not support write deadlines,
// because writes go through a plain blocking ReadWriteCloser.
func (p *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Close closes the connection.
// Any blocked ReadFrom or WriteTo operations will be unblocked and return errors.
func (p *Conn) Close() error {
	p.ctxCancel()
	return p.rwc.Close()
}

// getArenaBuf returns a buf from the packet arena with at least the given size.
func (p *Conn) getArenaBuf(size int) []byte {
	var buf []byte
	bufp := p.ar.Get()
	if bufp != nil {
		buf = *bufp.(*[]byte)
	}
	if size != 0 {
		if cap(buf) < size {
			buf = make([]byte, size)
		} else {
			buf = buf[:size]
		}
	} else {
		buf = buf[:cap(buf)]
	}
	return buf
}

// rxPump receives messages from the underlying connection.
func (p *Conn) rxPump() (rerr error) {
	defer func() {
		p.closeErr = rerr
		close(p.packetCh)
	}()

	// Read packets until cancellation or the underlying stream closes.
	for {
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}

		// Read into a reusable packet buffer and enqueue received bytes.
		pktBuf := p.getArenaBuf(int(connPktSize))
		n, err := p.rwc.Read(pktBuf)
		if n == 0 {
			p.ar.Put(&pktBuf)
		} else {
			select {
			case <-p.ctx.Done():
				return context.Canceled
			case p.packetCh <- pktBuf[:n]:
			}
		}
		if err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ net.Conn = (*Conn)(nil)
