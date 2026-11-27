//go:build !js

package main

import "github.com/aperturerobotics/cli"

// buildCommands constructs the command tree in CLI help order for this platform.
func buildCommands() []*cli.Command {
	return []*cli.Command{
		buildEnvelopeCommand(),
		buildPipeCommand(),
		buildClientCommand(),
		buildDaemonCommand(),
		buildUtilCommand(),
	}
}
