package main

import (
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"

	_ "github.com/aperturerobotics/controllerbus/example/boilerplate"
	_ "github.com/s4wave/spacewave/bldr/values"
)

// Commands are the CLI commands
var commands []*cli.Command

// Flags are the CLI flags
var flags []cli.Flag

// afterFuncs are cleanup functions called after the app exits.
var afterFuncs []func()

func main() {
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

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
