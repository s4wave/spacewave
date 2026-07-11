//go:build windows

package pipesock

import (
	"context"
	"net"

	util_pipesock "github.com/aperturerobotics/util/pipesock"
	"github.com/sirupsen/logrus"
)

// Listen creates a Windows named pipe owned by the returned listener.
func Listen(le *logrus.Entry, ownerDir, pipeUuid string) (*PipeListener, error) {
	listener, err := util_pipesock.BuildPipeListener(le, ownerDir, pipeUuid)
	if err != nil {
		return nil, err
	}
	return &PipeListener{
		Listener: listener,
		rootDir:  ownerDir,
		cleanup:  func() error { return nil },
	}, nil
}

// Dial connects to a pipe created by Listen.
func Dial(ctx context.Context, le *logrus.Entry, rootDir, pipeUuid string) (net.Conn, error) {
	return util_pipesock.DialPipeListener(ctx, le, rootDir, pipeUuid)
}
