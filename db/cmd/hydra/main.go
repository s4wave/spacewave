//go:build !js && !wasip1

package main

import (
	"os"

	"github.com/aperturerobotics/cli"
	hcli "github.com/s4wave/spacewave/db/cli"
	hydra_util "github.com/s4wave/spacewave/db/cli/util"
)

// main runs the Hydra client and daemon commands.
func main() {
	// Configure the Hydra command-line application.
	app := cli.NewApp()
	app.Name = "hydra"
	app.HideVersion = true
	app.Usage = "command-line node and tools for hydra"
	app.Commands = []*cli.Command{
		new(hcli.ClientArgs).BuildCommand(),
		new(daemonArgs).BuildCommand(),
		new(hydra_util.UtilArgs).BuildCommand(),
	}

	// Run the command and report failures to stderr.
	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}
