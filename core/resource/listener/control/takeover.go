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

// SocketInUseError reports that a live listener already holds the
// daemon socket and takeover was not requested. The condition is
// permanent for the refusing process: retrying without takeover
// cannot succeed while that listener lives.
type SocketInUseError struct {
	// Path is the contended socket path.
	Path string
}

// Error implements error.
func (e *SocketInUseError) Error() string {
	return "daemon socket " + e.Path + " is already in use"
}

// IsSocketInUse reports whether err wraps a SocketInUseError.
func IsSocketInUse(err error) bool {
	var inUse *SocketInUseError
	return errors.As(err, &inUse)
}

// TakeoverSocket ensures that the Unix socket at sockPath is free to be
// bound by requesting a live daemon to yield it or removing a stale file.
func TakeoverSocket(ctx context.Context, le *logrus.Entry, sockPath string) error {
	return prepareSocket(ctx, le, sockPath, true)
}

// EnsureSocketAvailable verifies that the Unix socket at sockPath is free to
// be bound. A live listener is an error; a stale socket file is removed.
func EnsureSocketAvailable(ctx context.Context, le *logrus.Entry, sockPath string) error {
	return prepareSocket(ctx, le, sockPath, false)
}

func prepareSocket(
	ctx context.Context,
	le *logrus.Entry,
	sockPath string,
	allowTakeover bool,
) error {
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

	if !allowTakeover {
		return &SocketInUseError{Path: sockPath}
	}

	if err := RequestShutdown(ctx, conn); err != nil {
		var denyErr *DenyError
		if errors.As(err, &denyErr) || ctx.Err() != nil {
			return err
		}

		// A yielded process can exit before its completion response
		// reaches us. Connection failure is the event; a fresh dial
		// distinguishes that completed exit from a still-live listener.
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
