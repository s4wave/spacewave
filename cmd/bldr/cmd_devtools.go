package main

import (
	bldr_dev "github.com/aperturerobotics/bldr/devtool"
)

var bldrFlags *bldr_dev.DevtoolArgs

func init() {
	bldrFlags = bldr_dev.NewDevtoolArgs()
	commands = append(commands, bldrFlags.BuildSubCommands()...)
	flags = append(flags, bldrFlags.BuildFlags()...)
}
