package main

import (
	bldr_cli "github.com/aperturerobotics/bldr/cli"
)

var bldrFlags bldr_cli.DevtoolArgs

func init() {
	commands = append(commands, bldrFlags.BuildSubCommands()...)
	flags = append(flags, bldrFlags.BuildFlags()...)
}
