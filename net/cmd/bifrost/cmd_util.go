package main

import (
	"github.com/aperturerobotics/cli"
	util "github.com/s4wave/spacewave/net/cli/util"
)

var utilArgs util.UtilArgs

func init() {
	utilCommands := (&utilArgs).BuildCommands()
	commands = append(
		commands,
		&cli.Command{
			Name:        "util",
			Usage:       "utility sub-commands",
			Subcommands: utilCommands,
			Flags:       (&utilArgs).BuildFlags(),
		},
	)
}
