//go:build linux

package boot

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_fuse "github.com/s4wave/spacewave/db/unixfs/fuse"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// consoleMount retains one FUSE server until its kernel mount is disconnected
// and Serve has joined. Storage is borrowed from RunConsole for that lifetime.
type consoleMount struct {
	// path stays bootstrap-side, outside both served roots.
	path string
	// fs owns the kernel connection and request server.
	fs *unixfs_fuse.RootFS
	// done publishes serveErr after the server exits.
	done chan struct{}
	// serveErr is read only after done closes.
	serveErr error
	// stopDisconnect unregisters the cancellation callback during ordered cleanup.
	stopDisconnect func() bool
	// disconnected joins any callback that already started.
	disconnected chan struct{}
}

// mountConsoleFS starts a retained server and disconnects blocked requests on
// cancellation. All helper execs finish before guest-wide waiting begins.
func mountConsoleFS(ctx context.Context, cancel context.CancelCauseFunc, stopping *atomic.Bool, le *logrus.Entry, path string, handle *unixfs.FSHandle, readOnly bool) (*consoleMount, error) {
	// Keep normal request service alive through ordinary unmount and flush.
	fs, err := unixfs_fuse.Mount(context.WithoutCancel(ctx), le, path, handle, false, readOnly, nil)
	if err != nil {
		return nil, err
	}
	m := &consoleMount{path: path, fs: fs, done: make(chan struct{}), disconnected: make(chan struct{})}
	m.stopDisconnect = context.AfterFunc(ctx, func() {
		fs.Close()
		close(m.disconnected)
	})

	// Unexpected exits must wake containment even when Serve returns nil.
	go func() {
		m.serveErr = fs.Serve()
		fs.Close()
		if !stopping.Load() {
			cancel(errors.Join(errors.New("console FUSE server stopped: "+path), m.serveErr))
		}
		close(m.done)
	}()
	return m, nil
}

// close unmounts before joining the server, preserving ordinary-unmount errors
// even when a detach succeeds. The caller has contained every guest process.
func (m *consoleMount) close() error {
	// Disconnect a failed mount so kernel waiters cannot pin cleanup indefinitely.
	err := unix.Unmount(m.path, 0)
	if err != nil {
		err = errors.Join(err, unix.Unmount(m.path, unix.MNT_DETACH))
		m.fs.Close()
	}

	// Join both the server and a cancellation callback before releasing storage.
	<-m.done
	if !m.stopDisconnect() {
		<-m.disconnected
	}
	return errors.Join(err, m.serveErr)
}
