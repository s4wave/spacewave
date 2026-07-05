//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/sdk/cli/runner"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

const statusMountSessionTimeoutEnvVar = "SPACEWAVE_STATUS_MOUNT_TIMEOUT"

var defaultStatusMountSessionTimeout = 10 * time.Second

// newStatusCommand builds the status CLI command.
func newStatusCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return runner.NewStatusCommand(nativeRunnerConfig())
}

// getStatusMountSessionTimeout returns the configured status mount bound.
func getStatusMountSessionTimeout() (time.Duration, error) {
	raw := os.Getenv(statusMountSessionTimeoutEnvVar)
	if raw == "" {
		return defaultStatusMountSessionTimeout, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.Wrap(err, statusMountSessionTimeoutEnvVar)
	}
	return dur, nil
}

func (s *nativeSession) WatchRecoveryStatus(ctx context.Context) (*s4wave_status.RecoveryStatus, error) {
	client, err := s.GetResourceRef().GetClient()
	if err != nil {
		return nil, err
	}
	strm, err := s4wave_status.NewSRPCSystemStatusServiceClient(client).WatchRecoveryStatus(
		ctx,
		&s4wave_status.WatchRecoveryStatusRequest{},
	)
	if err != nil {
		return nil, err
	}
	defer strm.Close()
	resp, err := strm.Recv()
	if err != nil {
		return nil, err
	}
	return resp.GetStatus(), nil
}
