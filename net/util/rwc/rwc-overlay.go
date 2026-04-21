package rwc

import (
	"io"
	"net"
	"time"
)

// RwcOverlay overlays a io.ReadWriteCloser on a net.Conn.
type RwcOverlay struct {
	nc  net.Conn
	rwc io.ReadWriteCloser
}

// NewRwcOverlay builds a ReadWriteCloser overlayed on a net.Conn.
func NewRwcOverlay(rwc io.ReadWriteCloser, nc net.Conn) *RwcOverlay {
	return &RwcOverlay{
		nc:  nc,
		rwc: rwc,
	}
}

// Read reads data from the connection.
// Read can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetReadDeadline.
func (r *RwcOverlay) Read(b []byte) (n int, err error) {
	return r.rwc.Read(b)
}

// Write writes data to the connection.
// Write can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetWriteDeadline.
func (r *RwcOverlay) Write(b []byte) (n int, err error) {
	return r.rwc.Write(b)
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (r *RwcOverlay) Close() error {
	err := r.rwc.Close()
	err2 := r.nc.Close()
	if err == nil && err2 != nil {
		err = err2
	}
	return err
}

// LocalAddr returns the local network address, if known.
func (r *RwcOverlay) LocalAddr() net.Addr {
	return r.nc.LocalAddr()
}

// RemoteAddr returns the remote network address, if known.
func (r *RwcOverlay) RemoteAddr() net.Addr {
	return r.nc.RemoteAddr()
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
// the deadline after successful Read or Write calls.
//
// A zero value for t means I/O operations will not time out.
func (r *RwcOverlay) SetDeadline(t time.Time) error {
	return r.nc.SetDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (r *RwcOverlay) SetReadDeadline(t time.Time) error {
	return r.nc.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (r *RwcOverlay) SetWriteDeadline(t time.Time) error {
	return r.nc.SetWriteDeadline(t)
}

// _ is a type assertion
var _ net.Conn = ((*RwcOverlay)(nil))
