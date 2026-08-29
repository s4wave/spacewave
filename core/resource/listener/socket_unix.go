//go:build !windows && !js

package resource_listener

import (
	"net"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// ListenProtectedUnix restricts the socket parent and socket before returning
// a listener. Managed parents are tightened to 0700. Explicit parents must
// already be 0700, except that a missing parent is created with that mode.
func ListenProtectedUnix(path string, managed bool) (*net.UnixListener, error) {
	return listenProtectedUnix(path, managed, os.Chmod)
}

func listenProtectedUnix(path string, managed bool, chmodSocket func(string, os.FileMode) error) (*net.UnixListener, error) {
	// Create a missing parent with owner-only access.
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Wrap(err, "inspect socket parent")
		}
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return nil, errors.Wrap(err, "create private socket parent")
		}
		info, err = os.Stat(parent)
		if err != nil {
			return nil, errors.Wrap(err, "inspect created socket parent")
		}
	}

	// Require or establish the parent policy before binding.
	if !info.IsDir() {
		return nil, errors.Errorf("socket parent %s is not a directory", parent)
	}
	if managed {
		if err := os.Chmod(parent, 0o700); err != nil {
			return nil, errors.Wrap(err, "protect socket parent")
		}
	} else if info.Mode().Perm() != 0o700 {
		return nil, errors.Errorf("socket parent %s is not private (mode %04o); choose a private directory (0700) for the explicit socket path", parent, info.Mode().Perm())
	}

	// Bind only after the parent blocks group and other traversal.
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}

	// Restrict the socket before returning it to an RPC server.
	if err := chmodSocket(path, 0o600); err != nil {
		_ = lis.Close()
		return nil, errors.Wrap(err, "protect Unix socket")
	}
	return lis, nil
}
