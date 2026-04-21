package logconn

import (
	"io"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

// LogConn wraps a net.Conn with a logger.
type LogConn struct {
	le         *logrus.Entry
	underlying net.Conn
}

func NewLogConn(le *logrus.Entry, underlying net.Conn) *LogConn {
	return &LogConn{
		le:         le,
		underlying: underlying,
	}
}

// Read reads data from the connection.
// Read can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetReadDeadline.
func (c *LogConn) Read(b []byte) (n int, err error) {
	n, err = c.underlying.Read(b)
	if err != nil && !(err == io.EOF && n != 0) { //nolint:staticcheck
		c.le.Warnf("read(...) => error %v", err.Error())
	} else {
		c.le.Debugf("read(...) => %v", string(b[:n]))
	}
	return
}

// Write writes data to the connection.
// Write can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetWriteDeadline.
func (c *LogConn) Write(b []byte) (n int, err error) {
	c.le.Debugf("write(%d): %v", len(b), string(b))
	n, err = c.underlying.Write(b)
	if err != nil {
		c.le.Warnf("write(%d) => errored %v", len(b), err.Error())
	}
	return
}

// LocalAddr returns the local network address.
func (c *LogConn) LocalAddr() net.Addr {
	return c.underlying.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *LogConn) RemoteAddr() net.Addr {
	return c.underlying.RemoteAddr()
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
func (c *LogConn) SetDeadline(t time.Time) error {
	return nil
	// return c.underlying.SetDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (c *LogConn) SetReadDeadline(t time.Time) error {
	// return c.underlying.SetReadDeadline(t)
	return nil
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (c *LogConn) SetWriteDeadline(t time.Time) error {
	// return c.underlying.SetWriteDeadline(t)
	return nil
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (c *LogConn) Close() error {
	err := c.underlying.Close()
	if err != nil {
		c.le.Warnf("close() => %v", err.Error())
	} else {
		c.le.Debugf("close() => success")
	}
	return err
}

// _ is a type assertion
var _ net.Conn = ((*LogConn)(nil))
