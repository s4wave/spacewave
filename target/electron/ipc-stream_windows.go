//go:build windows
// +build windows

package electron

import (
	"net"
	"path"

	"github.com/Microsoft/go-winio"
)

// buildPipeListener builds the pipe listener.
func buildPipeListener(electronRoot, sessionUuid string) (net.Listener, error) {
	pipeName := path.Join(electronRoot, ".pipe", sessionUuid)
	return winio.ListenPipe(pipeName, nil)
}
