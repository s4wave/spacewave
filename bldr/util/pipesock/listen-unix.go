//go:build !windows

package pipesock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"

	util_pipesock "github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const maxSocketPathLength = 90

// ListenProtectedUnix binds a Unix socket only after its parent is private and
// restricts the socket before returning it. The parent is created or tightened
// to 0700.
func ListenProtectedUnix(path string) (*net.UnixListener, error) {
	return listenProtectedUnix(path, os.Chmod)
}

func listenProtectedUnix(path string, chmodSocket func(string, os.FileMode) error) (*net.UnixListener, error) {
	// Establish owner-only traversal before creating the socket.
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

	// Restrict the parent before binding the socket.
	if !info.IsDir() {
		return nil, errors.Errorf("socket parent %s is not a directory", parent)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return nil, errors.Wrap(err, "protect socket parent")
	}

	// Bind only after the parent blocks group and other traversal.
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	// Restrict the socket before returning it to the RPC server.
	if err := chmodSocket(path, 0o600); err != nil {
		_ = lis.Close()
		return nil, errors.Wrap(err, "protect Unix socket")
	}
	return lis, nil
}

// Listen creates a short, private Unix socket owned by the returned listener.
func Listen(le *logrus.Entry, ownerDir, pipeUuid string) (*PipeListener, error) {
	ownerDir, err := filepath.Abs(ownerDir)
	if err != nil {
		return nil, errors.Wrap(err, "resolve pipe owner directory")
	}
	if pipeUuid == "" || filepath.Base(pipeUuid) != pipeUuid {
		return nil, errors.New("invalid pipe UUID")
	}

	sum := sha256.Sum256([]byte(filepath.Clean(ownerDir)))
	prefix := "bldr-" + hex.EncodeToString(sum[:6]) + "-"
	rootDir, err := os.MkdirTemp(shortTempDir(), prefix)
	if err != nil {
		return nil, errors.Wrap(err, "create private pipe directory")
	}
	cleanup := func() error { return os.RemoveAll(rootDir) }
	if err := os.Chmod(rootDir, 0o700); err != nil {
		_ = cleanup()
		return nil, errors.Wrap(err, "secure pipe directory")
	}

	path := filepath.Join(rootDir, ".pipe-"+pipeUuid)
	if len(path) > maxSocketPathLength {
		_ = cleanup()
		return nil, errors.Errorf("unix socket path exceeds %d bytes: %s", maxSocketPathLength, path)
	}
	listener, err := util_pipesock.BuildPipeListener(le, rootDir, pipeUuid)
	if err != nil {
		_ = cleanup()
		return nil, err
	}
	return &PipeListener{
		Listener: listener,
		rootDir:  rootDir,
		path:     path,
		cleanup:  cleanup,
	}, nil
}

// Dial connects to a pipe created by Listen.
func Dial(ctx context.Context, le *logrus.Entry, rootDir, pipeUuid string) (net.Conn, error) {
	return util_pipesock.DialPipeListener(ctx, le, rootDir, pipeUuid)
}

func shortTempDir() string {
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		return "/tmp"
	}
	return os.TempDir()
}
