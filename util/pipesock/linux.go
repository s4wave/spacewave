//go:build !windows
// +build !windows

package pipesock

import (
	"context"
	"net"
	"os"
	"path"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildPipeListener builds the pipe listener in the directory.
func BuildPipeListener(le *logrus.Entry, rootDir, pipeUuid string) (net.Listener, error) {
	pipePath := path.Join(rootDir, ".pipe-"+pipeUuid)

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
	pipePath := path.Join(rootDir, ".pipe-"+pipeUuid)
	addr := &net.UnixAddr{
		Net:  "unix",
		Name: pipePath,
	}
	le.Debugf("connecting to unix socket: %s", addr.String())
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, "unix", addr.String())
}
