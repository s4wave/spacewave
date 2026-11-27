//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/starpc/srpc"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
)

// daemonControlHandler owns the daemon's shutdown request. A granted
// Shutdown or Yield RPC, or a desktop Quit, requests shutdown from one
// connection: the request releases the listener, so admission stops, and
// records the requester, whose reply the serve loop waits for before it drains.
// DaemonShutdown.tla models this lifecycle with the connections it drains.
type daemonControlHandler struct {
	// Handler serves explicit daemon shutdown requests.
	*listener_control.Handler
	// release closes the accepting listener.
	release func()
	// requested closes when a shutdown is requested.
	requested chan struct{}
	// requestOnce closes requested once.
	requestOnce sync.Once
	// shutdownConn identifies the approved Shutdown requester.
	shutdownConn atomic.Pointer[trackedConn]
	// desktopQuitConn identifies the Quit requester whose RPC reply must drain.
	desktopQuitConn atomic.Pointer[trackedConn]
}

// newDaemonControlHandler constructs a daemon control handler that requests
// shutdown when the peer issues a granted Shutdown or Yield RPC. release must
// synchronously close the accepting listener. The CLI daemon always yields to
// an explicit takeover, but yields to a manual start only after SetYieldPolicy
// allows the manual start.
func newDaemonControlHandler(release func()) *daemonControlHandler {
	return newDaemonControlHandlerWithPolicy(listener_control.AutoAllowPolicy, release)
}

// newDaemonControlHandlerWithPolicy constructs a daemon control handler whose
// Shutdown RPC follows policy and records the granted requester's connection.
func newDaemonControlHandlerWithPolicy(
	policy listener_control.YieldPolicy,
	release func(),
) *daemonControlHandler {
	// Route the RPC's shutdown through the same request as desktop Quit.
	h := &daemonControlHandler{release: release, requested: make(chan struct{})}
	h.Handler = listener_control.NewHandler(policy, h.requestShutdown)

	// Record the granted requester before the handler releases the listener.
	h.SetShutdownGrantedCallback(func(ctx context.Context) {
		if tc, ok := ctx.Value(daemonConnCtxKey{}).(*trackedConn); ok {
			h.shutdownConn.Store(tc)
		}
	})
	return h
}

// requestShutdown marks shutdown requested and releases the listener.
func (h *daemonControlHandler) requestShutdown() {
	h.requestOnce.Do(func() { close(h.requested) })
	if h.release != nil {
		h.release()
	}
}

// requestDesktopQuit requests shutdown for a desktop Quit from requester,
// whose Quit reply the serve loop lets requester read before draining.
func (h *daemonControlHandler) requestDesktopQuit(requester *trackedConn) {
	h.desktopQuitConn.Store(requester)
	h.requestShutdown()
}

// waitReplied waits until the shutdown requester has read its reply and
// closed, or ctx ends. For a Shutdown RPC it first waits for the handler's
// acknowledgement. It returns at once when no shutdown was requested.
func (h *daemonControlHandler) waitReplied(ctx context.Context) error {
	// Return when the listener stopped for another reason.
	select {
	case <-h.requested:
	default:
		return nil
	}

	// A desktop Quit's requester closes after its RPC reply, even if the shell
	// exited during the RPC.
	if requester := h.desktopQuitConn.Load(); requester != nil {
		return requester.wait(ctx)
	}

	// An explicit Shutdown has a separate acknowledgement fence.
	if err := h.WaitShutdownComplete(ctx); err != nil {
		return err
	}
	if requester := h.shutdownConn.Load(); requester != nil {
		return requester.wait(ctx)
	}
	return nil
}

// requestDaemonShutdown issues the Shutdown RPC over conn.
func requestDaemonShutdown(ctx context.Context, conn net.Conn) error {
	return listener_control.RequestShutdown(ctx, conn)
}

// _ is a type assertion
var _ srpc.Handler = (*daemonControlHandler)(nil)
