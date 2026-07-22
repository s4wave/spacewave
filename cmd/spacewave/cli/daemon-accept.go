//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
)

// daemonConnCtxKey keys the accepting connection on each served stream context
// so the daemon-control handler can record which connection carried an approved
// Shutdown request.
type daemonConnCtxKey struct{}

// trackedConn decrements the idle tracker once when the connection closes and
// closes done when its serving goroutine exits.
type trackedConn struct {
	net.Conn
	done      chan struct{}
	closeOnce sync.Once
	onClose   func()
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
// daemon-control Shutdown closes the accepting listener or serveCtx is
// canceled. On an approved shutdown it waits for the control handler to finish
// writing its acknowledgement (ShutdownComplete) and then for the requester to
// read that acknowledgement and close its own connection, so the requester
// observes a clean stream close instead of a reset. Only after the requester
// departs does it drain any remaining clients and cancel the connection
// lifecycle. External serveCtx cancellation and accept errors still exit
// promptly.
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
	go func() {
		<-serveCtx.Done()
		_ = lis.Close()
	}()

	closeClients, err := acceptDaemonListener(serveCtx, lis, srv, idleTracker)
	serveCanceled := serveCtx.Err() != nil
	select {
	case <-shutdownCh:
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
	closeClients()
	serveCancel()
	if err != nil && (serveCanceled || errors.Is(err, net.ErrClosed)) {
		return nil
	}
	return err
}

// acceptDaemonListener accepts incoming daemon connections and tracks their
// lifecycle. It returns a drain function once accepting stops so the owner can
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

		if idleTracker != nil {
			idleTracker.clientAttached()
		}
		tc := &trackedConn{
			Conn: nc,
			done: make(chan struct{}),
			onClose: func() {
				if idleTracker != nil {
					idleTracker.clientDetached()
				}
			},
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
