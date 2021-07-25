package main

import (
	"os"

	"github.com/urfave/cli"
)

var Commands []cli.Command

func main() {
	app := cli.NewApp()
	app.Name = "sandbox"
	app.Usage = "app and ui development sandbox"
	app.HideVersion = true
	app.Author = "Christian Stewart <christian@aperturerobotics.com>"
	app.Commands = Commands

	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		_, _ = os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}
