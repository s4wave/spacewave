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
		if stderrors.Is(err, os.ErrNotExist) {
			os.Stdout.WriteString("No Spacewave daemon is running.\n")
			return nil
		}
		if stderrors.Is(err, syscall.ECONNREFUSED) {
			if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
				return errors.Wrap(err, "remove stale daemon socket")
			}
			os.Stdout.WriteString("No Spacewave daemon is running; removed a stale daemon socket.\n")
			return nil
		}
		return errors.Wrapf(err, "connect to %s", sockPath)
	}
	defer conn.Close()

	if err := requestDaemonShutdown(ctx, conn); err != nil {
		return errors.Wrap(err, "stop daemon")
	}
	os.Stdout.WriteString("Stopped Spacewave daemon.\n")
	return nil
}
