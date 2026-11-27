package main

import (
	"github.com/aperturerobotics/cli"
	bcli "github.com/s4wave/spacewave/net/cli"
)

// cliArgs are the client arguments.
var cliArgs bcli.ClientArgs

func init() {
	// Assemble client commands and connect the controller bus client.
	clientCommands := (&cliArgs).BuildCommands()
	clientFlags := (&cliArgs).BuildFlags()
	cbusCmd := cliArgs.BuildControllerBusCommand()

	// Register the client command tree with its shared connection flags.
	clientCommands = append(clientCommands, cbusCmd)
	commands = append(
		commands,
		&cli.Command{
			Name:        "client",
			Usage:       "client sub-commands",
			Subcommands: clientCommands,
			Flags:       clientFlags,
		},
	)
}
