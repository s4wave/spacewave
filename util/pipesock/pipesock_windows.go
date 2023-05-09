//go:build windows
// +build windows

package pipesock

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
)

// BuildPipeListener builds the pipe listener in the directory.
func BuildPipeListener(le *logrus.Entry, rootDir, pipeUuid string) (net.Listener, error) {
	pipeName := BuildPipeName(rootDir, pipeUuid)
	le.Debugf("listening on winio pipe: %s", pipeName)
	return winio.ListenPipe(pipeName, nil)
}

// DialPipeListener connects to the pipe listener in the directory.
func DialPipeListener(ctx context.Context, le *logrus.Entry, rootDir, pipeUuid string) (net.Conn, error) {
	pipeName := BuildPipeName(rootDir, pipeUuid)
	le.Debugf("connecting to winio pipe: %s", pipeName)
	return winio.DialPipeContext(ctx, pipeName)
}

// BuildPipeName builds a unique pipe name from a path and uuid.
// uuid must be unique for rootDir
func BuildPipeName(rootDir, pipeUuid string) string {
	return `\\.\pipe\bldr\` + pipeUuid
}
