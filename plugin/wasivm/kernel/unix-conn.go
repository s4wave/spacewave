package kernel

import (
	"context"
	"io"
	"net"
	"path"
	"time"

	"github.com/pkg/errors"
)

// errNoDeadline is returned by the deadline methods of a host connection.
var errNoDeadline = errors.New("unix connection deadlines are unsupported")

// DialUnix connects host code to the Unix stream socket listening at the
// absolute path in the guest's filesystem, as a guest's connect does. It waits
// for a socket to listen there, or for ctx to end. The guest accepts the
// connection like any other.
func (k *Kernel) DialUnix(ctx context.Context, socketPath string) (*UnixConn, error) {
	// Queue a connection once a listener is bound at the path. Each listen
	// wakes the wait.
	resolved := path.Clean("/" + socketPath)
	k.mu.Lock()
	of := k.newSocketFile(0)
	for k.connectUnix(of, hostCred, resolved) != 0 {
		var listenCh <-chan struct{}
		k.listens.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			listenCh = getWaitCh()
		})
		k.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-listenCh:
		}
		k.mu.Lock()
	}
	k.mu.Unlock()
	return k.newUnixConn(of, resolved), nil
}

// UnixConn is the host's end of a connection to a guest Unix socket. Its open
// file is never installed in a process, so only its pipes are shared.
type UnixConn struct {
	// k is the kernel whose lock guards the open files in flight.
	k *Kernel
	// of holds the pipes the connection reads and writes.
	of *OpenFile
	// path is the address of the guest's socket.
	path string
	// ctx ends when the connection closes, which ends a waiting read or
	// write.
	ctx    context.Context
	cancel context.CancelFunc
}

// newUnixConn wraps the host's end of the connection of to the socket at
// socketPath.
func (k *Kernel) newUnixConn(of *OpenFile, socketPath string) *UnixConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &UnixConn{k: k, of: of, path: socketPath, ctx: ctx, cancel: cancel}
}

// Read reads from the guest, waiting for data, and closes any files the
// guest sent with it. It returns io.EOF once the guest closes its end.
func (c *UnixConn) Read(b []byte) (int, error) {
	n, files, err := c.ReadMsg(b)
	for _, f := range files {
		_ = f.Close()
	}
	return n, err
}

// ReadMsg reads from the guest like Read, stopping before bytes that carry
// other files, and returns the files sent with the first byte read. The
// caller closes each file.
func (c *UnixConn) ReadMsg(b []byte) (int, []*File, error) {
	// Read the data and wrap the files.
	n, ofs, eof := c.of.rx.read(c.ctx, b)
	var files []*File
	for _, of := range ofs {
		files = append(files, &File{k: c.k, of: of})
	}

	// Report end of file or a close.
	switch {
	case n != 0:
		return n, files, nil
	case eof:
		return 0, nil, io.EOF
	case c.ctx.Err() != nil:
		return 0, nil, net.ErrClosed
	}
	return 0, nil, nil
}

// Write writes to the guest, waiting for room. It returns io.ErrClosedPipe
// once the guest closes its end.
func (c *UnixConn) Write(b []byte) (int, error) {
	return c.WriteMsg(b, nil)
}

// WriteMsg writes to the guest like Write and sends files with the first
// byte written. The caller keeps its references to files. A write of no
// bytes sends no files.
func (c *UnixConn) WriteMsg(b []byte, files []*File) (int, error) {
	// Take a reference to each file for the pipe.
	var ofs []*OpenFile
	if len(files) != 0 {
		c.k.mu.Lock()
		for _, f := range files {
			f.of.ref()
			ofs = append(ofs, f.of)
		}
		c.k.mu.Unlock()
	}

	// Write, releasing the references when nothing was written.
	n, ok := c.of.tx.write(c.ctx, b, ofs)
	if n == 0 && len(ofs) != 0 {
		c.k.mu.Lock()
		unrefFiles(ofs)
		c.k.mu.Unlock()
	}

	// Report a closed guest or connection.
	switch {
	case !ok:
		return 0, io.ErrClosedPipe
	case n < len(b):
		return n, net.ErrClosed
	}
	return n, nil
}

// Close closes both directions, which the guest reads as end of file, and
// ends a waiting read or write.
func (c *UnixConn) Close() error {
	// End the waits, then release the pipes and the files in flight under
	// the kernel lock.
	c.cancel()
	c.k.mu.Lock()
	c.of.release()
	c.k.mu.Unlock()
	return nil
}

// LocalAddr returns the host's end, which has no address.
func (c *UnixConn) LocalAddr() net.Addr {
	return &net.UnixAddr{Net: "unix"}
}

// RemoteAddr returns the guest socket's path.
func (c *UnixConn) RemoteAddr() net.Addr {
	return &net.UnixAddr{Name: c.path, Net: "unix"}
}

// SetDeadline is unsupported. Close ends a waiting read or write.
func (c *UnixConn) SetDeadline(time.Time) error {
	return errNoDeadline
}

// SetReadDeadline is unsupported.
func (c *UnixConn) SetReadDeadline(time.Time) error {
	return errNoDeadline
}

// SetWriteDeadline is unsupported.
func (c *UnixConn) SetWriteDeadline(time.Time) error {
	return errNoDeadline
}

// _ is a type assertion
var _ net.Conn = (*UnixConn)(nil)
