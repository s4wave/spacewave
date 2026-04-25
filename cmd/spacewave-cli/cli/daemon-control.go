//go:build !js

package spacewave_cli

import (
	"context"
	"net"

	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
)

// newDaemonControlHandler constructs a daemon control handler that
// invokes requestShutdown when the peer issues the Shutdown RPC.
// The CLI daemon always yields when asked; the policy is AutoAllow.
func newDaemonControlHandler(requestShutdown func()) *listener_control.Handler {
	return listener_control.NewHandler(listener_control.AutoAllowPolicy, requestShutdown)
}

// requestDaemonShutdown issues the Shutdown RPC over conn.
func requestDaemonShutdown(ctx context.Context, conn net.Conn) error {
	return listener_control.RequestShutdown(ctx, conn)
}
