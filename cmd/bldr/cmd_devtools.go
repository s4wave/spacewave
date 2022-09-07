package main

import (
	bldr_cli "github.com/aperturerobotics/bldr/cli"
)

var bldrFlags *bldr_cli.DevtoolArgs

func init() {
	bldrFlags = bldr_cli.NewDevtoolArgs()
	commands = append(commands, bldrFlags.BuildSubCommands()...)
	flags = append(flags, bldrFlags.BuildFlags()...)
}
