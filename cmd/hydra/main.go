//go:build !js && !wasip1
// +build !js,!wasip1

package main

import (
	"os"

	"github.com/urfave/cli/v2"
)

// Commands are the CLI commands
var commands []*cli.Command

func main() {
	app := cli.NewApp()
	app.Name = "hydra"
	app.HideVersion = true
	app.Usage = "command-line node and tools for hydra"
	app.Commands = commands

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}
