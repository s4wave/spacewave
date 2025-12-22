//go:build !windows

package pipesock

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildPipeListener builds the pipe listener.
// The rootDir is used for unix sockets if this is a linux system.
// The pipeUuid is used for the socket path OR the Windows Pipe Name.
// The pipeUuid should be unique to the local device and pipe.
func BuildPipeListener(le *logrus.Entry, rootDir, pipeUuid string) (net.Listener, error) {
	// Create absolute path for the socket
	absolutePipePath := filepath.Join(rootDir, ".pipe-"+pipeUuid)

	// Ensure the parent directory exists
	pipeDir := filepath.Dir(absolutePipePath)
	if err := os.MkdirAll(pipeDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create pipe directory")
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		// If we can't get CWD (e.g., it was deleted), use absolute path
		le.WithError(err).Debug("could not get cwd, using absolute pipe path")
		cwd = ""
	}

	// Get relative path from current working directory if possible
	// Use whichever is shorter (Unix socket paths are limited to ~104 chars)
	pipePath := absolutePipePath
	if cwd != "" {
		relPath, err := filepath.Rel(cwd, absolutePipePath)
		if err == nil && len(relPath) < len(absolutePipePath) {
			pipePath = relPath
		}
	}

	// remove old pipe file, if exists
	if _, err := os.Stat(pipePath); !os.IsNotExist(err) {
		if err := os.Remove(pipePath); err != nil {
			return nil, errors.Wrap(err, "remove old pipe file")
		}
	}

	addr := &net.UnixAddr{
		Net:  "unix",
		Name: pipePath,
	}
	le.Debugf("listening on unix socket: %s", addr.String())
	return net.ListenUnix("unix", addr)
}

// DialPipeListener connects to the pipe listener in the directory.
func DialPipeListener(ctx context.Context, le *logrus.Entry, rootDir, pipeUuid string) (net.Conn, error) {
	// Create absolute path for the socket
	absolutePipePath := filepath.Join(rootDir, ".pipe-"+pipeUuid)

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		// If we can't get CWD (e.g., it was deleted), use absolute path
		le.WithError(err).Debug("could not get cwd, using absolute pipe path")
		cwd = ""
	}

	// Get relative path from current working directory if possible
	// Use whichever is shorter (Unix socket paths are limited to ~104 chars)
	pipePath := absolutePipePath
	if cwd != "" {
		relPath, err := filepath.Rel(cwd, absolutePipePath)
		if err == nil && len(relPath) < len(absolutePipePath) {
			pipePath = relPath
		}
	}

	addr := &net.UnixAddr{
		Net:  "unix",
		Name: pipePath,
	}
	le.Debugf("connecting to unix socket: %s (cwd: %s)", addr.String(), cwd)
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, "unix", addr.String())
}
