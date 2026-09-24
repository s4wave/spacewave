//go:build !js

// Package spacewave_cli provides CLI commands for the spacewave CLI binary.
package spacewave_cli

import (
	"fmt"
	"math"

	"github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	hydra_cli "github.com/s4wave/spacewave/db/cli"
	hydra_cliutil "github.com/s4wave/spacewave/db/cli/util"
	bifrost_cli "github.com/s4wave/spacewave/net/cli"
	bifrost_cliutil "github.com/s4wave/spacewave/net/cli/util"
	"github.com/s4wave/spacewave/sdk/cli/runner"
)

// NewCliCommands builds the spacewave CLI commands. The yield broker is the
// process-shared broker from the composition root; nil builds commands with
// a private broker for tests and read-only command trees.
func NewCliCommands(getBus func() cli_entrypoint.CliBus, yieldBroker *yield_policy.Broker) []*cli.Command {
	if yieldBroker == nil {
		yieldBroker = yield_policy.NewBroker()
	}
	commands := []*cli.Command{
		// Tier 1: entry points
		newLoginCommand(getBus),
		newLogoutCommand(getBus),
		newWhoamiCommand(getBus),
		newServeCommand(getBus, yieldBroker),
		newStopCommand(getBus),
		newStatusCommand(getBus),
		newWebCommand(getBus),
	}
	commands = appendTuiCommand(commands)
	return append(
		commands,
		// Tier 2: auth
		newAuthCommand(getBus),

		// Tier 3: data operations
		newBillingCommand(getBus),
		newSpaceCommand(getBus),
		newDeviceCommand(getBus),
		newStorageCommand(getBus),
		newFsCommand(getBus),
		newGitCommand(getBus),
		newCanvasCommand(getBus),
		newAptCommand(getBus),
		newForgeCommand(getBus),
		newVmCommand(getBus),
		newPluginCommand(getBus),
		newDebugCommand(getBus),
		newBifrostCommand(),
		newHydraCommand(),

		// Tier 4: plumbing
		newAccountCommand(getBus),
		newSessionCommand(getBus),
		newProviderCommand(getBus),
	)
}

func nativeRunnerConfig() runner.Config {
	return runner.Config{
		ClientFactory:       nativeClientFactory{},
		ClientFlags:         nativeRunnerClientFlags,
		MountSessionTimeout: getStatusMountSessionTimeout,
	}
}

func nativeRunnerClientFlags(sessionIdx *uint) []cli.Flag {
	return clientFlags(nil, sessionIdx)
}

// newBifrostCommand embeds the bifrost CLI command set.
func newBifrostCommand() *cli.Command {
	var clientArgs bifrost_cli.ClientArgs
	var utilArgs bifrost_cliutil.UtilArgs
	return &cli.Command{
		Name:  "bifrost",
		Usage: "Bifrost network-router sub-commands.",
		Subcommands: append(
			[]*cli.Command{{
				Name:        "util",
				Usage:       "utility sub-commands",
				Subcommands: utilArgs.BuildCommands(),
				Flags:       utilArgs.BuildFlags(),
				Before: func(c *cli.Context) error {
					utilArgs.SetContext(c.Context)
					return nil
				},
			}},
			clientArgs.BuildCommands()...,
		),
		Flags: clientArgs.BuildFlags(),
		Before: func(c *cli.Context) error {
			clientArgs.SetContext(c.Context)
			return nil
		},
	}
}

// newHydraCommand embeds the hydra storage CLI command set.
func newHydraCommand() *cli.Command {
	var clientArgs hydra_cli.ClientArgs
	var utilArgs hydra_cliutil.UtilArgs
	cmd := clientArgs.BuildHydraCommand()
	cmd.Subcommands = append(cmd.Subcommands, &cli.Command{
		Name:        "util",
		Usage:       "utility sub-commands",
		Subcommands: utilArgs.BuildCommands(),
		Flags:       utilArgs.BuildFlags(),
		Before: func(c *cli.Context) error {
			utilArgs.SetContext(c.Context)
			return nil
		},
	})
	cmd.Flags = clientArgs.BuildFlags()
	cmd.Before = func(c *cli.Context) error {
		clientArgs.SetContext(c.Context)
		return nil
	}
	return cmd
}

// clientFlags returns the common flags for client commands.
func clientFlags(statePath *string, sessionIdx *uint) []cli.Flag {
	return []cli.Flag{
		statePathFlag(statePath),
		socketPathFlag(),
		&cli.UintFlag{
			Name:        "session-index",
			Usage:       "session index to use",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Value:       1,
			Destination: sessionIdx,
			Action: func(_ *cli.Context, value uint) error {
				if value > math.MaxUint32 {
					return fmt.Errorf("session-index exceeds uint32 range: %d", value)
				}
				return nil
			},
		},
	}
}

func sessionIndex32(value uint) uint32 {
	if value > math.MaxUint32 {
		panic("session index must be validated before conversion")
	}
	return uint32(value) //nolint:gosec // the range check enforces the daemon's uint32 session-index API.
}

func sessionIndexFromInt(value int) (uint32, error) {
	if value <= 0 || uint64(value) > math.MaxUint32 {
		return 0, fmt.Errorf("session index out of range: %d", value)
	}
	return uint32(value), nil //nolint:gosec // the positive MaxUint32 check bounds the URI session index.
}

// daemonClientFlags returns common flags for daemon clients that do not select a session.
func daemonClientFlags(statePath *string) []cli.Flag {
	return []cli.Flag{
		statePathFlag(statePath),
		socketPathFlag(),
	}
}

// statePathFlag returns the common --state-path flag.
func statePathFlag(dest *string) cli.Flag {
	return &cli.StringFlag{
		Name:        "state-path",
		Aliases:     []string{"s"},
		Usage:       "daemon state directory path",
		EnvVars:     statePathEnvVars,
		Value:       defaultStatePath,
		Destination: dest,
	}
}

// socketPathFlag returns the common connect-only daemon socket flag.
func socketPathFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "socket-path",
		Usage:   "connect to an existing daemon socket at this exact path",
		EnvVars: socketPathEnvVars,
	}
}
