package main

import (
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"
)

var version string

func main() {
	// Assemble the Bifrost CLI with its registered commands.
	app := cli.NewApp()
	app.Name = "bifrost"
	app.HideVersion = true
	app.Usage = "command-line node and tools for bifrost"
	app.Commands = buildCommands()

	// Metadata
	app.Description = "Check out the docs and examples:\n\nhttps://github.com/s4wave/spacewave/net"
	app.Copyright = "Apache License, Version 2.0."

	// Hide version if unset
	if version == "" {
		app.HideVersion = true
	} else {
		app.Version = version
	}

	// Run the Bifrost command and report failures to the invoking shell.
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
