//go:build windows

package pipesock

import (
	"context"
	"net"
	"os"
	"path/filepath"

	util_pipesock "github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ListenProtectedUnix preserves Windows Unix-socket behavior. Windows does
// not use Unix mode bits as filesystem authority, but protection failures still
// fail closed before the listener is returned.
func ListenProtectedUnix(path string) (*net.UnixListener, error) {
	// Preserve the existing Windows parent and Unix-socket construction.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errors.Wrap(err, "create socket parent")
	}

	// Fail before returning when the platform rejects socket protection.
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

// Listen creates a Windows named pipe owned by the returned listener.
func Listen(le *logrus.Entry, ownerDir, pipeUuid string) (*PipeListener, error) {
	listener, err := util_pipesock.BuildPipeListener(le, ownerDir, pipeUuid)
	if err != nil {
		return nil, err
	}
	return &PipeListener{
		Listener: listener,
		rootDir:  ownerDir,
		cleanup:  func() error { return nil },
	}, nil
}

// Dial connects to a pipe created by Listen.
func Dial(ctx context.Context, le *logrus.Entry, rootDir, pipeUuid string) (net.Conn, error) {
	return util_pipesock.DialPipeListener(ctx, le, rootDir, pipeUuid)
}
