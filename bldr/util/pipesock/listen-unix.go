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
