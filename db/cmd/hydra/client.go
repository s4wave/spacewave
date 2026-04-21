//go:build !js && !wasip1

package main

import (
	"github.com/aperturerobotics/cli"
	hcli "github.com/s4wave/spacewave/db/cli"
)

var clientArgs hcli.ClientArgs

func init() {
	clientCommands := (&clientArgs).BuildCommands()

	// controller-bus
	cbusCmd := (&clientArgs.CbusConf).BuildControllerBusCommand()
	cbusCmd.Before = func(_ *cli.Context) error {
		client, err := (&clientArgs).BuildClient()
		if err != nil {
			return err
		}
		(&clientArgs.CbusConf).SetClient(client)
		return nil
	}
	clientCommands = append(clientCommands, cbusCmd)

	// bifrost
	bifrostCmd := (&clientArgs.BifrostConf).BuildBifrostCommand()
	bifrostCmd.Before = func(_ *cli.Context) error {
		client, err := (&clientArgs).BuildClient()
		if err != nil {
			return err
		}
		(&clientArgs.BifrostConf).SetClient(client)
		return nil
	}
	clientCommands = append(clientCommands, bifrostCmd)

	commands = append(
		commands,
		&cli.Command{
			Name:        "client",
			Usage:       "client sub-commands",
			Subcommands: clientCommands,
			Flags:       (&clientArgs).BuildFlags(),
		},
	)
}
