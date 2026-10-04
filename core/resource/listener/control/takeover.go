//go:build !js

package control

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"syscall"

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
// handedOff reports that a live daemon acknowledged the request and released
// the socket; its runtime may still be releasing its other resources. A peer
// that disconnects without acknowledging reports no handoff.
func TakeoverSocket(
	ctx context.Context,
	le *logrus.Entry,
	sockPath string,
) (handedOff bool, err error) {
	return prepareSocket(ctx, le, sockPath, true)
}

// EnsureSocketAvailable verifies that the Unix socket at sockPath is free to
// be bound. A live listener is an error; a stale socket file is removed.
func EnsureSocketAvailable(ctx context.Context, le *logrus.Entry, sockPath string) error {
	_, err := prepareSocket(ctx, le, sockPath, false)
	return err
}

// prepareSocket protects a live listener unless explicit takeover is
// accepted, and reports whether a live listener handed off the socket.
func prepareSocket(
	ctx context.Context,
	le *logrus.Entry,
	sockPath string,
	allowTakeover bool,
) (handedOff bool, err error) {
	// Inspect the daemon socket path before attempting a handoff.
	socketInfo, err := os.Stat(sockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "stat daemon socket")
	}

	// A non-socket path may contain user data even when connect returns refusal.
	if socketInfo.Mode()&os.ModeSocket == 0 {
		return false, errors.Errorf("daemon socket path is not a socket: %s", sockPath)
	}

	// Watch before attempting handoff so socket release cannot go unnoticed.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return false, errors.Wrap(err, "watch daemon socket handoff")
	}
	defer watcher.Close()
	if err := watcher.Add(filepath.Dir(sockPath)); err != nil {
		return false, errors.Wrap(err, "watch daemon socket directory")
	}

	// Connect to the daemon or reclaim a socket whose listener is gone.
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
	if err != nil {
		// Only a refused connection proves a stale socket; permission and other
		// transport errors leave the existing path untouched.
		if !errors.Is(err, syscall.ECONNREFUSED) && !errors.Is(err, os.ErrNotExist) {
			return false, errors.Wrap(err, "connect to daemon socket")
		}
		le.WithError(err).Warn("removing stale daemon socket")
		if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
			return false, errors.Wrap(err, "remove stale daemon socket")
		}
		return false, nil
	}
	defer conn.Close()

	// Protect a live daemon listener when takeover was not requested.
	if !allowTakeover {
		return false, &SocketInUseError{Path: sockPath}
	}

	// Request daemon shutdown and confirm listener release after a peer exit.
	if err := RequestShutdown(ctx, conn); err != nil {
		var denyErr *DenyError
		if errors.As(err, &denyErr) || ctx.Err() != nil {
			return false, err
		}

		// A yielded process can exit before its completion response
		// reaches us. Connection failure is the event; a fresh dial
		// distinguishes that completed exit from a still-live listener.
		_ = conn.Close()
		probe, probeErr := (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
		if probeErr == nil {
			_ = probe.Close()
			return false, err
		}
		if !errors.Is(probeErr, syscall.ECONNREFUSED) && !errors.Is(probeErr, os.ErrNotExist) {
			return false, errors.Wrap(probeErr, "confirm daemon socket release")
		}
		le.WithError(err).Warn("takeover peer exited before handoff completion; removing stale daemon socket")
		if removeErr := os.Remove(sockPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return false, errors.Wrap(removeErr, "remove stale daemon socket after peer exit")
		}
		return false, nil
	}
	if err := waitForSocketRelease(ctx, watcher, socketInfo, sockPath); err != nil {
		return false, err
	}
	return true, nil
}

// waitForSocketRelease observes pathname changes after an accepted handoff.
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
