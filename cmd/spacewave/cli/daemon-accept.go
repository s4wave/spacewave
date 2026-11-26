//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"runtime/pprof"
	"strconv"
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
	// peer is the executable name of the connecting process, if known.
	peer string
	// pid is the process ID of the connecting process, or zero if unknown.
	pid int
	// done closes when the serving goroutine exits.
	done chan struct{}
	// closeOnce ensures the idle hold is released once.
	closeOnce sync.Once
	// onClose releases the admitted client's idle hold.
	onClose func()
}

// newTrackedConn wraps an accepted connection under its peer process name.
func newTrackedConn(nc net.Conn) *trackedConn {
	pid, peer := peerProcess(nc)
	return &trackedConn{Conn: nc, peer: peer, pid: pid, done: make(chan struct{})}
}

// admit attaches the connection to idleTracker, which holds the daemon alive
// until the connection closes. It reports false once shutdown is claimed. A nil
// idleTracker admits every connection.
func (c *trackedConn) admit(idleTracker *daemonIdleTracker) bool {
	if idleTracker == nil {
		return true
	}
	if !idleTracker.trackedClientAttached(c) {
		return false
	}
	c.onClose = func() { idleTracker.trackedClientDetached(c) }
	return true
}

// serve serves the connection's streams on srv until the connection ends,
// then closes it and marks it done.
func (c *trackedConn) serve(ctx context.Context, srv *srpc.Server, mc srpc.MuxedConn) {
	defer close(c.done)
	defer c.Close()
	// Goroutines serving this client inherit the peer label, so a CPU profile
	// attributes their work with -tagfocus=peer=name:pid.
	pprof.Do(ctx, pprof.Labels("peer", c.profileLabel()), func(ctx context.Context) {
		_ = srv.AcceptMuxedConn(ctx, mc)
	})
}

// wait waits until the connection's serving goroutine exits or ctx ends.
func (c *trackedConn) wait(ctx context.Context) error {
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// describe names the connection to a person waiting on it.
func (c *trackedConn) describe() string {
	if c.peer == "" {
		return unnamedClient
	}
	return c.peer
}

// profileLabel names the connection in CPU profiles as "name:pid", so a profile
// can tell apart several processes running the same executable.
func (c *trackedConn) profileLabel() string {
	return c.describe() + ":" + strconv.Itoa(c.pid)
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
// accepting listener. After a Shutdown or Quit it waits for the requester to
// read its reply and close. Only then does it drain remaining clients and
// cancel their lifecycle. External serveCtx cancellation and accept errors
// still exit promptly.
func serveDaemonListener(
	serveCtx context.Context,
	serveCancel context.CancelFunc,
	lis net.Listener,
	srv *srpc.Server,
	controlHandler *daemonControlHandler,
	idleTracker *daemonIdleTracker,
) error {
	// Stop accepting when the serving context ends.
	go func() {
		<-serveCtx.Done()
		_ = lis.Close()
	}()

	// Preserve the shutdown reply before draining clients.
	closeClients, err := acceptDaemonListener(serveCtx, lis, srv, idleTracker)
	serveCanceled := serveCtx.Err() != nil
	_ = controlHandler.waitReplied(serveCtx)

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
	// Track admitted connections so the drain can close and await them.
	var clients sync.WaitGroup
	var connsMtx sync.Mutex
	conns := make(map[*trackedConn]struct{})
	closeClients := func() {
		// Close tracked connections and wait for clients to finish.
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

	// Serve clients until the listener closes.
	for {
		// Accept the next client.
		nc, err := lis.Accept()
		if err != nil {
			return closeClients, err
		}

		// Admit the client under its peer process name, or reject it once
		// shutdown is claimed.
		tc := newTrackedConn(nc)
		if !tc.admit(idleTracker) {
			_ = nc.Close()
			continue
		}
		mc, err := srpc.NewMuxedConn(tc, false, nil)
		if err != nil {
			_ = tc.Close()
			continue
		}

		// Serve the client, dropping it from the drain set once it ends.
		connsMtx.Lock()
		conns[tc] = struct{}{}
		connsMtx.Unlock()
		connCtx := context.WithValue(ctx, daemonConnCtxKey{}, tc)
		clients.Go(func() {
			defer func() {
				connsMtx.Lock()
				delete(conns, tc)
				connsMtx.Unlock()
			}()
			tc.serve(connCtx, srv, mc)
		})
	}
}
