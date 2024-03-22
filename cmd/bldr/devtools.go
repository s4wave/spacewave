package main

import (
	bldr_dev "github.com/aperturerobotics/bldr/devtool"
)

var devtoolArgs *bldr_dev.DevtoolArgs

func init() {
	devtoolArgs = bldr_dev.NewDevtoolArgs()
	commands = append(commands, devtoolArgs.BuildSubCommands()...)
	flags = append(flags, devtoolArgs.BuildFlags()...)
}
