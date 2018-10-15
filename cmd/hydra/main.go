package main

import (
	"github.com/urfave/cli"
	"os"
)

var commands []cli.Command
var flags []cli.Flag

func main() {
	app := cli.NewApp()

	app.HideVersion = true
	app.Name = "hydra"
	app.Usage = "hydra process and controllers"
	app.Commands = commands
	app.Flags = flags

	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		_, _ = os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}
