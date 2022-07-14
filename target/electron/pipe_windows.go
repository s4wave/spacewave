//go:build windows
// +build windows

package electron

import (
	"net"
	"path"

	"github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
)

// buildPipeListener builds the pipe listener in the working directory.
func buildPipeListener(le *logrus.Entry, electronRoot, sessionUuid string) (net.Listener, error) {
	pipeName := path.Join(electronRoot, ".pipe", sessionUuid)
	le.Debugf("listening for ipc: %s", sessionUuid)
	return winio.ListenPipe(pipeName, nil)
}
