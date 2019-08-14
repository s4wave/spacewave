package main

import (
	hcli "github.com/aperturerobotics/hydra/cli"
	"github.com/urfave/cli"
)

var clientArgs hcli.ClientArgs

func init() {
	clientCommands := (&clientArgs).BuildCommands()
	commands = append(
		commands,
		cli.Command{
			Name:        "client",
			Usage:       "client sub-commands",
			Subcommands: clientCommands,
			Flags:       (&clientArgs).BuildFlags(),
		},
	)
}
