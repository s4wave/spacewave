package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	_ "github.com/aperturerobotics/controllerbus/example/boilerplate"
)

// Commands are the CLI commands
var commands []*cli.Command

// Flags are the CLI flags
var flags []cli.Flag

func main() {
	app := cli.NewApp()
	app.Name = "bldr"
	app.HideVersion = true
	app.Usage = "cross-platform application bundle and deploy"
	app.Commands = commands
	app.Flags = flags

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
