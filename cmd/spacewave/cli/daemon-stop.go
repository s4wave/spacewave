//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
)

// newStopCommand builds the daemon stop command.
func newStopCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	return &cli.Command{
		Name:  "stop",
		Usage: "stop the daemon",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
		},
		Action: func(c *cli.Context) error {
			resolved, err := resolveStatePathFromContext(c, statePath)
			if err != nil {
				return err
			}
			return runStop(c.Context, resolved)
		},
	}
}

func runStop(ctx context.Context, statePath string) error {
	sockPath := filepath.Join(statePath, socketName)
	conn, err := connectDaemonDial(ctx, sockPath)
	if err != nil {
		if handled, handleErr := handleUnavailableDaemonSocket(sockPath, err); handled {
			return handleErr
		}
		return errors.Wrapf(err, "connect to %s", sockPath)
	}
	defer conn.Close()

	if err := requestDaemonShutdown(ctx, conn); err != nil {
		var denyErr *listener_control.DenyError
		if stderrors.As(err, &denyErr) || ctx.Err() != nil {
			return errors.Wrap(err, "stop daemon")
		}

		_ = conn.Close()
		probe, probeErr := connectDaemonDial(ctx, sockPath)
		if probeErr == nil {
			_ = probe.Close()
			return errors.Wrap(err, "stop daemon")
		}
		if handled, handleErr := handleUnavailableDaemonSocket(sockPath, probeErr); handled {
			return handleErr
		}
		return errors.Wrapf(err, "stop daemon; verify listener exit: %v", probeErr)
	}
	os.Stdout.WriteString("Stopped Spacewave daemon.\n")
	return nil
}

// handleUnavailableDaemonSocket reports errors that prove no daemon listener
// remains. Other dial errors must remain fatal to the stop command.
func handleUnavailableDaemonSocket(sockPath string, dialErr error) (bool, error) {
	switch {
	case stderrors.Is(dialErr, os.ErrNotExist):
		os.Stdout.WriteString("No Spacewave daemon is running.\n")
		return true, nil
	case stderrors.Is(dialErr, syscall.ECONNREFUSED):
		if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
			return true, errors.Wrap(err, "remove stale daemon socket")
		}
		os.Stdout.WriteString("No Spacewave daemon is running; removed a stale daemon socket.\n")
		return true, nil
	default:
		return false, nil
	}
}
