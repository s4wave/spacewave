//go:build !js

package spacewave_cli

import (
	"github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/sdk/cli/runner"
)

// newWhoamiCommand builds the whoami command.
func newWhoamiCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return runner.NewWhoamiCommand(nativeRunnerConfig())
}
