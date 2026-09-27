//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// daemonConnCtxKey keys the accepting connection on each served stream context
// so the daemon-control handler can record which connection carried an approved
// Shutdown request.
type daemonConnCtxKey struct{}

// trackedConn decrements the idle tracker once when the connection closes and
// closes done when its serving goroutine exits.
type trackedConn struct {
	net.Conn
	// done closes when the serving goroutine exits.
	done chan struct{}
	// closeOnce ensures the idle hold is released once.
	closeOnce sync.Once
	// onClose releases the admitted client's idle hold.
	onClose func()
}

// Close closes the connection and runs the close callback once.
func (c *trackedConn) Close() error {
	c.closeOnce.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})
	return c.Conn.Close()
}

// serveDaemonListener serves accepted daemon clients until an approved
// daemon-control Shutdown, desktop Quit, or serveCtx cancellation closes the
// accepting listener. On an approved Shutdown it waits for the control handler
// acknowledgement. On desktop Quit it waits for the requester to read its RPC
// reply and close. Only then does it drain remaining clients and cancel their
// lifecycle. External serveCtx cancellation and accept errors still exit promptly.
//
// The shutdown callback registered on controlHandler must close only the
// accepting listener and signal shutdownCh; it must not cancel serveCtx.
func serveDaemonListener(
	serveCtx context.Context,
	serveCancel context.CancelFunc,
	lis net.Listener,
	srv *srpc.Server,
	controlHandler *daemonControlHandler,
	shutdownCh <-chan struct{},
	idleTracker *daemonIdleTracker,
) error {
	// Stop accepting when the serving context ends.
	go func() {
		<-serveCtx.Done()
		_ = lis.Close()
	}()

	// Preserve the shutdown acknowledgement before draining clients.
	closeClients, err := acceptDaemonListener(serveCtx, lis, srv, idleTracker)
	serveCanceled := serveCtx.Err() != nil
	select {
	case <-shutdownCh:
		// Preserve the desktop Quit stream until its requester receives the
		// reply and closes, even if the shell exited during the RPC.
		if requester := controlHandler.desktopQuitConn.Load(); requester != nil {
			select {
			case <-requester.done:
			case <-serveCtx.Done():
			}
			break
		}

		// Explicit Shutdown has a separate acknowledgement fence.
		select {
		case <-controlHandler.ShutdownComplete():
		case <-serveCtx.Done():
		}
		if requester := controlHandler.shutdownConn.Load(); requester != nil {
			select {
			case <-requester.done:
			case <-serveCtx.Done():
			}
		}
	default:
	}

	// Drain connections and normalize an expected listener shutdown.
	closeClients()
	serveCancel()
	if serveCanceled || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// acceptDaemonListener accepts incoming daemon connections and tracks their
// lifecycle. It returns a drain function once accepting stops so the caller can
// release the listener, let a shutdown requester read its acknowledgement, and
// only then close any remaining clients synchronously.
func acceptDaemonListener(
	ctx context.Context,
	lis net.Listener,
	srv *srpc.Server,
	idleTracker *daemonIdleTracker,
) (func(), error) {
	var clients sync.WaitGroup
	var connsMtx sync.Mutex
	conns := make(map[*trackedConn]struct{})
	closeClients := func() {
		connsMtx.Lock()
		active := make([]*trackedConn, 0, len(conns))
		for conn := range conns {
			active = append(active, conn)
		}
		connsMtx.Unlock()
		for _, conn := range active {
			_ = conn.Close()
		}
		clients.Wait()
	}

	for {
		nc, err := lis.Accept()
		if err != nil {
			return closeClients, err
		}

		tc := &trackedConn{
			Conn: nc,
			done: make(chan struct{}),
		}
		if idleTracker != nil {
			if !idleTracker.trackedClientAttached(tc) {
				_ = nc.Close()
				continue
			}
			tc.onClose = func() { idleTracker.trackedClientDetached(tc) }
		}

		mc, err := srpc.NewMuxedConn(tc, false, nil)
		if err != nil {
			_ = tc.Close()
			continue
		}

		connsMtx.Lock()
		conns[tc] = struct{}{}
		connsMtx.Unlock()

		connCtx := context.WithValue(ctx, daemonConnCtxKey{}, tc)
		clients.Go(func() {
			defer close(tc.done)
			defer func() {
				connsMtx.Lock()
				delete(conns, tc)
				connsMtx.Unlock()
			}()
			defer tc.Close()
			_ = srv.AcceptMuxedConn(connCtx, mc)
		})
	}
}
