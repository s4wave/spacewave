//go:build !windows
// +build !windows

package electron

import (
	"net"
	"os"
	"path"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// buildPipeListener builds the pipe listener in the working directory.
func buildPipeListener(le *logrus.Entry, rootDir, runtimeUuid string) (net.Listener, error) {
	pipePath := path.Join(rootDir, ".pipe-"+runtimeUuid)

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
	le.Debugf("listening for ipc: %s", addr.String())
	return net.ListenUnix("unix", addr)
}
