//go:build !windows
// +build !windows

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
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "get current working directory")
	}

	// Create absolute path for the socket
	absolutePipePath := filepath.Join(rootDir, ".pipe-"+pipeUuid)

	// Get relative path from current working directory
	pipePath, err := filepath.Rel(cwd, absolutePipePath)
	if err != nil {
		return nil, errors.Wrap(err, "get relative path for pipe")
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
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "get current working directory")
	}

	// Create absolute path for the socket
	absolutePipePath := filepath.Join(rootDir, ".pipe-"+pipeUuid)

	// Get relative path from current working directory
	pipePath, err := filepath.Rel(cwd, absolutePipePath)
	if err != nil {
		return nil, errors.Wrap(err, "get relative path for pipe")
	}

	addr := &net.UnixAddr{
		Net:  "unix",
		Name: pipePath,
	}
	le.Debugf("connecting to unix socket: %s (relative to %s)", addr.String(), cwd)
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, "unix", addr.String())
}
