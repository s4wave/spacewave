//go:build !js && !wasip1

package main

import (
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"
)

// Commands are the CLI commands
var commands []*cli.Command

func main() {
	// Configure the forge command-line application.
	app := cli.NewApp()
	app.Name = "forge"
	app.HideVersion = true
	app.Usage = "distributed task scheduling system"
	app.Commands = commands

	// Run the application and report command failures.
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
