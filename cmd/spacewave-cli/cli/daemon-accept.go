//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
)

// trackedConn decrements the idle tracker once when the connection closes.
type trackedConn struct {
	net.Conn
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

// acceptDaemonListener accepts incoming daemon connections and tracks their lifecycle.
func acceptDaemonListener(ctx context.Context, lis net.Listener, srv *srpc.Server, idleTracker *daemonIdleTracker) error {
	for {
		nc, err := lis.Accept()
		if err != nil {
			return err
		}

		if idleTracker != nil {
			idleTracker.clientAttached()
		}
		tc := &trackedConn{
			Conn: nc,
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

		if err := srv.AcceptMuxedConn(ctx, mc); err != nil {
			_ = tc.Close()
			continue
		}
	}
}
