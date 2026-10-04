//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"sync/atomic"

	"github.com/aperturerobotics/starpc/srpc"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
)

// daemonControlHandler wraps the shared daemon-control handler and records the
// granted connection so the serve loop can wait for that requester to read its
// acknowledgement and close before draining.
type daemonControlHandler struct {
	// Handler serves explicit daemon shutdown requests.
	*listener_control.Handler
	// shutdownConn identifies the approved Shutdown requester.
	shutdownConn atomic.Pointer[trackedConn]
	// desktopQuitConn identifies the Quit requester whose RPC reply must drain.
	desktopQuitConn atomic.Pointer[trackedConn]
}

// newDaemonControlHandler constructs a daemon control handler that invokes
// requestShutdown when the peer issues a granted Shutdown or Yield RPC. The
// CLI daemon always yields to an explicit takeover; it yields to a manual
// start only after SetYieldPolicy allows it.
func newDaemonControlHandler(requestShutdown func()) *daemonControlHandler {
	return newDaemonControlHandlerWithPolicy(listener_control.AutoAllowPolicy, requestShutdown)
}

// newDaemonControlHandlerWithPolicy constructs a daemon control handler whose
// Shutdown RPC follows policy and records the granted requester's connection.
func newDaemonControlHandlerWithPolicy(
	policy listener_control.YieldPolicy,
	requestShutdown func(),
) *daemonControlHandler {
	h := &daemonControlHandler{
		Handler: listener_control.NewHandler(policy, requestShutdown),
	}
	h.SetShutdownGrantedCallback(func(ctx context.Context) {
		if tc, ok := ctx.Value(daemonConnCtxKey{}).(*trackedConn); ok {
			h.shutdownConn.Store(tc)
		}
	})
	return h
}

// requestDaemonShutdown issues the Shutdown RPC over conn.
func requestDaemonShutdown(ctx context.Context, conn net.Conn) error {
	return listener_control.RequestShutdown(ctx, conn)
}

// _ is a type assertion
var _ srpc.Handler = (*daemonControlHandler)(nil)
