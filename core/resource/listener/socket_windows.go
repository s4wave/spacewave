//go:build windows && !js

package resource_listener

import (
	"net"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// ListenProtectedUnix preserves Windows Unix-socket behavior. Windows does
// not use Unix mode bits as filesystem authority; callers must not treat the
// requested modes as an access-control boundary.
func ListenProtectedUnix(path string, managed bool) (*net.UnixListener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errors.Wrap(err, "create socket parent")
	}
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = lis.Close()
		return nil, errors.Wrap(err, "protect Unix socket")
	}
	return lis, nil
}
