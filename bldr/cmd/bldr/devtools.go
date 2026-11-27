//go:build !js

package main

import (
	"github.com/aperturerobotics/cli"
	bldr_dev "github.com/s4wave/spacewave/bldr/devtool"
)

// buildCommands constructs the native command tree and its shared flags and cleanup.
func buildCommands() ([]*cli.Command, []cli.Flag, []func()) {
	// Keep the debug command before the development commands in CLI help.
	devtoolArgs := bldr_dev.NewDevtoolArgs(nil)
	commands := append([]*cli.Command{buildDebugCommand()}, devtoolArgs.BuildSubCommands()...)

	// Bind flags and cleanup to the same development arguments as the commands.
	return commands, devtoolArgs.BuildFlags(), []func(){devtoolArgs.CloseLogFiles}
}
