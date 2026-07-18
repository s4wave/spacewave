//go:build windows

package launcher_helper

import (
	"net"

	"github.com/Microsoft/go-winio"
	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const helperPipeSecurityDescriptor = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;OW)"

func newHelperPipeListener(
	le *logrus.Entry,
	rootDir, pipeID string,
) (net.Listener, error) {
	pipeName := pipesock.BuildPipeName(rootDir, pipeID)
	le.Debugf("listening on private winio pipe: %s", pipeName)
	listener, err := winio.ListenPipe(pipeName, &winio.PipeConfig{
		SecurityDescriptor: helperPipeSecurityDescriptor,
	})
	if err != nil {
		return nil, errors.Wrap(err, "listen on private helper pipe")
	}
	return listener, nil
}
