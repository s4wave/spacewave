//go:build !js && !wasip1

package main

import (
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"
	fcli "github.com/s4wave/spacewave/forge/cli"
)

// main runs the Forge client and daemon commands.
func main() {
	// Configure the forge command-line application.
	app := cli.NewApp()
	app.Name = "forge"
	app.HideVersion = true
	app.Usage = "distributed task scheduling system"
	app.Commands = []*cli.Command{
		new(fcli.ClientArgs).BuildCommand(),
		new(daemonArgs).BuildCommand(),
	}

	// Run the application and report command failures.
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
