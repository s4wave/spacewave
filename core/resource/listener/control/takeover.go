//go:build !js

package control

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/fsnotify"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TakeoverSocket ensures that the Unix socket at sockPath is free to
// be bound. If a live daemon is already listening there, TakeoverSocket
// issues the daemon-control Shutdown RPC and waits for its completion
// acknowledgement. If the peer exits after yielding but before sending
// that acknowledgement, TakeoverSocket observes the closed connection,
// verifies that no listener remains, and removes the stale socket.
//
// Callers must bind directly after a successful return. They must not
// remove the path: the completed protocol or stale-owner recovery
// already released it, and a later remove could unlink a concurrent
// winner's listener.
func TakeoverSocket(ctx context.Context, le *logrus.Entry, sockPath string) error {
	socketInfo, err := os.Stat(sockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "stat daemon socket")
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "watch daemon socket handoff")
	}
	defer watcher.Close()
	if err := watcher.Add(filepath.Dir(sockPath)); err != nil {
		return errors.Wrap(err, "watch daemon socket directory")
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
		var denyErr *DenyError
		if errors.As(err, &denyErr) || ctx.Err() != nil {
			return err
		}

		// A yielded process can exit before its completion response
		// reaches us. Connection failure is the event; a fresh dial
		// distinguishes that completed exit from a still-live owner.
		_ = conn.Close()
		probe, probeErr := net.Dial("unix", sockPath)
		if probeErr == nil {
			_ = probe.Close()
			return err
		}
		le.WithError(err).Warn("takeover peer exited before handoff completion; removing stale daemon socket")
		if removeErr := os.Remove(sockPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return errors.Wrap(removeErr, "remove stale daemon socket after peer exit")
		}
		return nil
	}
	return waitForSocketRelease(ctx, watcher, socketInfo, sockPath)
}

func waitForSocketRelease(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	original os.FileInfo,
	sockPath string,
) error {
	for {
		current, err := os.Stat(sockPath)
		switch {
		case os.IsNotExist(err):
			return nil
		case err != nil:
			return errors.Wrap(err, "stat daemon socket after handoff completion")
		case !os.SameFile(original, current):
			return errors.New("daemon socket owner changed during takeover")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-watcher.Errors:
			return errors.Wrap(err, "watch daemon socket handoff")
		case event := <-watcher.Events:
			if event.Name != sockPath {
				continue
			}
		}
	}
}
