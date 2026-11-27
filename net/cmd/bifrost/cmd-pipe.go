//go:build !js

package main

import (
	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/net/cli/pipe"
)

// buildPipeCommand constructs the pipe command with its own arguments.
func buildPipeCommand() *cli.Command {
	var pipeArgs pipe.PipeArgs
	return &cli.Command{
		Name:   "pipe",
		Usage:  "pipe stdin/stdout over bifrost (netcat-like)",
		Action: pipeArgs.Run,
		Flags:  pipeArgs.BuildFlags(),
	}
}
