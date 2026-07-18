//go:build !windows

package launcher_helper

import (
	"net"

	"github.com/aperturerobotics/util/pipesock"
	"github.com/sirupsen/logrus"
)

func newHelperPipeListener(
	le *logrus.Entry,
	rootDir, pipeID string,
) (net.Listener, error) {
	return pipesock.BuildPipeListener(le, rootDir, pipeID)
}
