package main

import (
	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/net/cli/client"
)

// buildClientCommand constructs the client commands and their shared connection flags.
func buildClientCommand() *cli.Command {
	// Assemble client commands and connect the controller bus client.
	var cliArgs client.ClientArgs
	clientCommands := (&cliArgs).BuildCommands()
	clientFlags := (&cliArgs).BuildFlags()
	cbusCmd := cliArgs.BuildControllerBusCommand()

	// Register the client command tree with its shared connection flags.
	clientCommands = append(clientCommands, cbusCmd)
	return &cli.Command{
		Name:        "client",
		Usage:       "client sub-commands",
		Subcommands: clientCommands,
		Flags:       clientFlags,
	}
}
