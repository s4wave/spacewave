package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

// Commands are the CLI commands
var commands []cli.Command

func main() {
	app := cli.NewApp()
	app.Name = "hydra"
	app.HideVersion = true
	app.Usage = "command-line node and tools for hydra"
	app.Commands = commands

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
