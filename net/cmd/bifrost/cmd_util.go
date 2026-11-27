package main

import (
	"github.com/aperturerobotics/cli"
	util "github.com/s4wave/spacewave/net/cli/util"
)

// buildUtilCommand constructs the utility commands and their shared flags.
func buildUtilCommand() *cli.Command {
	var utilArgs util.UtilArgs
	utilCommands := (&utilArgs).BuildCommands()
	return &cli.Command{
		Name:        "util",
		Usage:       "utility sub-commands",
		Subcommands: utilCommands,
		Flags:       (&utilArgs).BuildFlags(),
	}
}
