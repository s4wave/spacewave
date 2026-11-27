package main

import (
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"

	_ "github.com/aperturerobotics/controllerbus/example/boilerplate"
	_ "github.com/s4wave/spacewave/bldr/values"
)

func main() {
	// Configure the Bldr command tree and its post-run cleanup.
	commands, flags, afterFuncs := buildCommands()
	app := cli.NewApp()
	app.Name = "bldr"
	app.HideVersion = true
	app.Usage = "cross-platform application bundle and deploy"
	app.Commands = commands
	app.Flags = flags
	app.After = func(c *cli.Context) error {
		for _, fn := range afterFuncs {
			fn()
		}
		return nil
	}

	// Run the Bldr command with the process arguments.
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
