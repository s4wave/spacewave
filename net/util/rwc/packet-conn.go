package rwc

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// PacketConn implements a PacketConn with a Read-Write-Closer.
//
// Writes a little-endian 4 byte length prefix before each packet.
// All messages sent to/from the wrong addresses are dropped.
type PacketConn struct {
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
	// maxPacketSize is the maximum allowed packet size
	maxPacketSize uint32

	ar       sync.Pool   // packet arena
	rd       time.Time   // read deadline
	wd       time.Time   // write deadline
	packetCh chan []byte // packet ch
	closeErr error
}

// NewPacketConn constructs a new packet conn and starts the rx pump.
func NewPacketConn(
	ctx context.Context,
	rwc io.ReadWriteCloser,
	laddr, raddr net.Addr,
	maxPacketSize uint32,
	bufferPacketN int,
) *PacketConn {
	ctx, ctxCancel := context.WithCancel(ctx)
	if bufferPacketN <= 0 {
		bufferPacketN = 10
	}

	c := &PacketConn{
		ctx:           ctx,
		ctxCancel:     ctxCancel,
		rwc:           rwc,
		laddr:         laddr,
		raddr:         raddr,
		maxPacketSize: maxPacketSize,
		packetCh:      make(chan []byte, bufferPacketN),
	}
	go func() {
		_ = c.rxPump()
	}()
	return c
}

// LocalAddr returns the local network address.
func (p *PacketConn) LocalAddr() net.Addr {
	return p.laddr
}

// RemoteAddr returns the bound remote network address.
func (p *PacketConn) RemoteAddr() net.Addr {
	return p.raddr
}

// ReadFrom reads a packet from the connection,
// copying the payload into p. It returns the number of
// bytes copied into p and the return address that
// was on the packet.
// It returns the number of bytes read (0 <= n <= len(p))
// and any error encountered. Callers should always process
// the n > 0 bytes returned before considering the error err.
// ReadFrom can be made to time out and return an error after a
// fixed time limit; see SetDeadline and SetReadDeadline.
func (p *PacketConn) ReadFrom(pk []byte) (n int, addr net.Addr, err error) {
	deadline := p.rd
	ctx := p.ctx
	if !deadline.IsZero() {
		var ctxCancel context.CancelFunc
		ctx, ctxCancel = context.WithDeadline(ctx, deadline)
		defer ctxCancel()
	}

	var pkt []byte
	var ok bool
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case pkt, ok = <-p.packetCh:
		if !ok {
			err = p.closeErr
			if err == nil {
				err = io.EOF
			}
			return 0, nil, err
		}
	}

	pl := len(pkt)
	copy(pk, pkt)
	p.ar.Put(&pkt)
	if len(pk) < pl {
		return len(pk), p.raddr, io.ErrShortBuffer
	}
	return pl, p.raddr, nil
}

// WriteTo writes a packet with payload p to addr.
// WriteTo can be made to time out and return an Error after a
// fixed time limit; see SetDeadline and SetWriteDeadline.
// On packet-oriented connections, write timeouts are rare.
func (p *PacketConn) WriteTo(pkt []byte, addr net.Addr) (n int, err error) {
	if len(pkt) == 0 {
		return 0, nil
	}

	if addr != p.raddr {
		as := addr.String()
		rs := p.raddr.String()
		if as != rs {
			return 0, errors.Errorf("packet conn bound to %s and cannot write to %s", rs, as)
		}
	}

	pktLen := len(pkt)
	if pktLen > math.MaxInt32 {
		return 0, errors.New("message too large: exceeds maximum uint32 value")
	}

	buf := p.getArenaBuf(pktLen + 4)
	binary.LittleEndian.PutUint32(buf, uint32(pktLen))

	copy(buf[4:], pkt)

	n, err = p.rwc.Write(buf)
	if err == nil && n < len(buf) {
		err = errors.Errorf("expected conn to write %d bytes in one call but wrote %d", len(buf), n)
	}

	p.ar.Put(&buf)
	if err != nil {
		return n, err
	}

	return n - 4, nil
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
func (p *PacketConn) SetDeadline(t time.Time) error {
	p.rd = t
	p.wd = t
	return nil
}

// SetReadDeadline sets the deadline for future ReadFrom calls
// and any currently-blocked ReadFrom call.
// A zero value for t means ReadFrom will not time out.
func (p *PacketConn) SetReadDeadline(t time.Time) error {
	p.rd = t
	return nil
}

// SetWriteDeadline sets the deadline for future WriteTo calls
// and any currently-blocked WriteTo call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means WriteTo will not time out.
func (p *PacketConn) SetWriteDeadline(t time.Time) error {
	p.wd = t
	return nil
}

// Close closes the connection.
// Any blocked ReadFrom or WriteTo operations will be unblocked and return errors.
func (p *PacketConn) Close() error {
	return p.rwc.Close()
}

// getArenaBuf returns a buf from the packet arena with at least the given size
func (p *PacketConn) getArenaBuf(size int) []byte {
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
func (p *PacketConn) rxPump() (rerr error) {
	defer func() {
		p.closeErr = rerr
		close(p.packetCh)
	}()

	var header [4]byte
	for {
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}

		_, err := io.ReadFull(p.rwc, header[:])
		if err != nil {
			return err
		}

		pktLen := binary.LittleEndian.Uint32(header[:])
		if pktLen == 0 {
			return errors.New("received header indicating zero packet size")
		}
		if pktLen > p.maxPacketSize {
			return errors.Errorf("indicated packet size %d larger than max size %d", pktLen, p.maxPacketSize)
		}

		pktBuf := p.getArenaBuf(int(pktLen))
		_, err = io.ReadFull(p.rwc, pktBuf)
		if err != nil {
			return err
		}
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		case p.packetCh <- pktBuf:
		}
	}
}

// _ is a type assertion
var _ net.PacketConn = ((*PacketConn)(nil))
