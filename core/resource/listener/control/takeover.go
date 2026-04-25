//go:build !js

package control

import (
	"context"
	"net"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TakeoverSocket ensures that the Unix socket at sockPath is free to
// be bound. If a live daemon is already listening there, TakeoverSocket
// issues the daemon-control Shutdown RPC over the existing socket,
// triggering the peer to close its listener and clean up. If a stale
// socket file is present but no daemon answers, the file is removed.
//
// Callers should follow a successful return with their own net.Listen
// on the same path. The peer is expected to remove the socket file on
// shutdown; on platforms where that is unreliable, callers may still
// need to os.Remove before re-binding.
func TakeoverSocket(ctx context.Context, le *logrus.Entry, sockPath string) error {
	if _, err := os.Stat(sockPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "stat daemon socket")
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		le.WithError(err).Warn("removing stale daemon socket")
		if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
			return errors.Wrap(err, "remove stale daemon socket")
		}
		return nil
	}
	defer conn.Close()

	if err := RequestShutdown(ctx, conn); err != nil {
		return err
	}
	return nil
}
