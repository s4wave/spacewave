//go:build !js

package main

import (
	"github.com/aperturerobotics/cli"
	bcli "github.com/s4wave/spacewave/net/cli"
)

// pipeArgs are the pipe command arguments.
var pipeArgs bcli.PipeArgs

func init() {
	commands = append(commands, &cli.Command{
		Name:   "pipe",
		Usage:  "pipe stdin/stdout over bifrost (netcat-like)",
		Action: pipeArgs.Run,
		Flags:  pipeArgs.BuildFlags(),
	})
}
