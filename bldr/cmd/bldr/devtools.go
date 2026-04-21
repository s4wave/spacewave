//go:build !js

package main

import (
	bldr_dev "github.com/s4wave/spacewave/bldr/devtool"
)

var devtoolArgs *bldr_dev.DevtoolArgs

func init() {
	devtoolArgs = bldr_dev.NewDevtoolArgs()
	commands = append(commands, devtoolArgs.BuildSubCommands()...)
	flags = append(flags, devtoolArgs.BuildFlags()...)
	afterFuncs = append(afterFuncs, devtoolArgs.CloseLogFiles)
}
