//go:build windows
// +build windows

package pipesock

import (
	"context"
	"net"
	"path"

	"github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
)

// BuildPipeListener builds the pipe listener in the directory.
func BuildPipeListener(le *logrus.Entry, rootDir, pipeUuid string) (net.Listener, error) {
	pipeName := path.Join(rootDir, ".pipe", pipeUuid)
	le.Debugf("listening on winio socket: %s", pipeUuid)
	return winio.ListenPipe(pipeName, nil)
}

// DialPipeListener connects to the pipe listener in the directory.
func DialPipeListener(ctx context.Context, le *logrus.Entry, rootDir, pipeUuid string) (net.Conn, error) {
	pipePath := path.Join(rootDir, ".pipe-"+pipeUuid)
	addr := &net.UnixAddr{
		Net:  "unix",
		Name: pipePath,
	}
	le.Debugf("connecting to winio socket: %s", addr.String())
	return winio.DialPipeContext(ctx, pipePath)
}
