//go:build !js

package main

import (
	"os"

	appcli "github.com/aperturerobotics/cli"

	debug_cli "github.com/s4wave/spacewave/cmd/spacewave-debug/cli"
)

var args debug_cli.ClientArgs

func main() {
	// Cancel debug commands when the process receives an interrupt.
	ctx, stop := args.NotifyInterrupt()
	defer stop()

	// Register and run the page inspection and debug commands.
	app := appcli.NewApp()
	app.Name = "spacewave-debug"
	app.HideVersion = true
	app.Usage = "debug bridge for interacting with a running Spacewave Alpha page"
	app.Commands = append(
		args.BuildCommands(),
		args.BuildForgeCommand(),
		args.BuildSpaceCommand(),
		args.BuildAccountCommand(),
		buildLinebreaksCommand(),
		buildOrphansCommand(),
		buildMeasureCommand(),
		buildGridCheckCommand(),
		buildPreviewTextCommand(),
		buildWatchCommand(),
	)
	if err := app.RunContext(ctx, os.Args); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
