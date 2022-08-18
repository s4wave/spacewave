package main

import (
	util "github.com/aperturerobotics/hydra/cli/util"
	"github.com/urfave/cli/v2"
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
