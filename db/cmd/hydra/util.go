//go:build !js && !wasip1

package main

import (
	"github.com/aperturerobotics/cli"
	util "github.com/s4wave/spacewave/db/cli/util"
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
