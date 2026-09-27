//go:build !js

package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"github.com/aperturerobotics/fsnotify"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
)

// SocketName is the Resource socket beneath a Spacewave state root.
const SocketName = "spacewave.sock"

// Connector resolves native daemon targets and converges concurrent launches
// through socket and readiness events. Only the daemon holding the state lease
// may bind and publish readiness.
type Connector struct {
	// dial opens a connection to an exact Unix socket path.
	dial func(context.Context, string) (net.Conn, error)
	// start returns after the child relinquishes startup custody, or ErrStarting
	// when another process holds the state lease. It cleans up its own failure.
	start func(context.Context, string) error
}

// NewConnector constructs a connector. Nil callbacks select the native dialer
// and detached executable launcher; callbacks allow isolated process fixtures.
func NewConnector(dial func(context.Context, string) (net.Conn, error), start func(context.Context, string) error) *Connector {
	// Supply the native transport and process boundaries by default.
	if dial == nil {
		dial = func(ctx context.Context, path string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		}
	}
	if start == nil {
		start = StartProcess
	}
	return &Connector{dial: dial, start: start}
}

// Dial connects to the resolved target. An explicit socket is connect-only and
// does not resolve or create a state root. Otherwise the root follows storagepath
// defaults and only absence or refusal permits starting a detached daemon.
// The returned connection still needs Resource Init to establish readiness.
func (c *Connector) Dial(ctx context.Context, statePath, socketPath string) (net.Conn, error) {
	// Environment socket selection has the same connect-only semantics as flags.
	if socketPath == "" {
		socketPath = os.Getenv(storagepath.SocketPathEnvVar("spacewave"))
	}

	// Explicit socket selection grants no process or writable-state authority.
	if socketPath != "" {
		conn, err := c.dial(ctx, socketPath)
		if err != nil {
			return nil, errors.Wrapf(err, "no daemon listening at %s; start the Spacewave desktop app or run spacewave serve with a matching --state-path", socketPath)
		}
		return conn, nil
	}

	// Resolve one absolute root before subscribing or starting the executable.
	if statePath == "" {
		var err error
		statePath, err = storagepath.DetermineStorageRoot("spacewave")
		if err != nil {
			return nil, err
		}
	}
	statePath, err := filepath.Abs(statePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(statePath, 0o700); err != nil {
		return nil, err
	}
	socketPath = filepath.Join(statePath, SocketName)

	// Subscribe before checking absence, including the lease loser's readback.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	defer watcher.Close()
	if err := watcher.Add(statePath); err != nil {
		return nil, err
	}
	timeout, err := StartupTimeout()
	if err != nil {
		return nil, err
	}
	startCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Permission and transport failures never authorize launching a writer.
	conn, err := c.dial(startCtx, socketPath)
	if err == nil {
		return conn, nil
	}
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ECONNREFUSED) {
		return nil, errors.Wrap(err, "connect to existing daemon socket")
	}
	if err := c.start(startCtx, statePath); err != nil && !errors.Is(err, ErrStarting) {
		return nil, errors.Wrap(err, "start daemon")
	}

	// Read after socket or readiness events. Binding can precede listen, so
	// pathname creation alone cannot wake every refused connection attempt.
	for {
		conn, err = c.dial(startCtx, socketPath)
		if err == nil {
			return conn, nil
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, err
		}
	wait:
		for {
			select {
			case <-startCtx.Done():
				return nil, startCtx.Err()
			case err := <-watcher.Errors:
				return nil, errors.Wrap(err, "watch daemon startup")
			case event := <-watcher.Events:
				if event.Name == socketPath || event.Name == socketPath+readinessSuffix {
					break wait
				}
			}
		}
	}
}

// Connect completes Resource Init on the resolved daemon. Closing the returned
// client releases only its connection, including when this call started it.
func (c *Connector) Connect(ctx context.Context, statePath, socketPath string) (*Client, error) {
	// Keep Resource streams tied to the caller, independently of startup expiry.
	conn, err := c.Dial(ctx, statePath, socketPath)
	if err != nil {
		return nil, err
	}
	return NewClient(ctx, conn)
}
