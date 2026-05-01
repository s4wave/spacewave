//go:build !js

// Package spacewave_cli provides CLI commands for the spacewave CLI binary.
package spacewave_cli

import (
	"github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	hydra_cli "github.com/s4wave/spacewave/db/cli"
	hydra_cliutil "github.com/s4wave/spacewave/db/cli/util"
	bifrost_cli "github.com/s4wave/spacewave/net/cli"
	bifrost_cliutil "github.com/s4wave/spacewave/net/cli/util"
)

// NewCliCommands builds the spacewave CLI commands.
func NewCliCommands(getBus func() cli_entrypoint.CliBus) []*cli.Command {
	return []*cli.Command{
		// Tier 1: entry points
		newLoginCommand(getBus),
		newLogoutCommand(getBus),
		newWhoamiCommand(getBus),
		newServeCommand(getBus),
		newStopCommand(getBus),
		newStatusCommand(getBus),
		newWebCommand(getBus),

		// Tier 2: auth
		newAuthCommand(getBus),

		// Tier 3: data operations
		newBillingCommand(getBus),
		newSpaceCommand(getBus),
		newFsCommand(getBus),
		newGitCommand(getBus),
		newCanvasCommand(getBus),
		newForgeCommand(getBus),
		newVmCommand(getBus),
		newPluginCommand(getBus),
		newBifrostCommand(),
		newHydraCommand(),

		// Tier 4: plumbing
		newAccountCommand(getBus),
		newSessionCommand(getBus),
		newProviderCommand(getBus),
	}
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
		&cli.StringFlag{
			Name:    "socket-path",
			Usage:   "connect to an existing daemon socket at this exact path",
			EnvVars: socketPathEnvVars,
		},
		&cli.UintFlag{
			Name:        "session-index",
			Usage:       "session index to use",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Value:       1,
			Destination: sessionIdx,
		},
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
