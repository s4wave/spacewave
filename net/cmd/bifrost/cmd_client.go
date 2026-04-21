package main

import (
	"github.com/aperturerobotics/cli"
	bcli "github.com/s4wave/spacewave/net/cli"
)

// cliArgs are the client arguments
var cliArgs bcli.ClientArgs

func init() {
	clientCommands := (&cliArgs).BuildCommands()
	clientFlags := (&cliArgs).BuildFlags()
	cbusCmd := (&cliArgs.CbusConf).BuildControllerBusCommand()
	cbusCmd.Before = func(_ *cli.Context) error {
		client, err := (&cliArgs).BuildClient()
		if err != nil {
			return err
		}
		(&cliArgs.CbusConf).SetClient(client)
		return nil
	}
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
