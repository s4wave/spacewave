//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/pkg/errors"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
	"github.com/sirupsen/logrus"
)

// takeoverDaemonSocket shuts down an existing daemon listener or
// removes a stale socket so a new listener can bind to sockPath, and
// reports whether a live listener handed off the socket. A DenyError
// from the peer is wrapped with guidance mentioning the Spacewave
// desktop app so the CLI error message is actionable.
func takeoverDaemonSocket(
	ctx context.Context,
	le *logrus.Entry,
	sockPath string,
) (bool, error) {
	// Take over the daemon socket, or explain a denial.
	handedOff, err := listener_control.TakeoverSocket(ctx, le, sockPath)
	if err == nil {
		return handedOff, nil
	}
	var denyErr *listener_control.DenyError
	if errors.As(err, &denyErr) {
		return false, errors.Errorf(
			"Spacewave desktop app denied the takeover request: %s. Quit the Spacewave desktop app or approve the prompt in-app before retrying.",
			denyErr.Error(),
		)
	}
	return false, err
}
