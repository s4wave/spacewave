//go:build !js && !wasip1

// Command spacewave-recovery counts surviving evidence in an isolated capture.
package main

import (
	"os"

	"github.com/aperturerobotics/cli"
)

// main runs the count or durable report command.
func main() {
	// Run the same parser and action as the persisted capture run-through.
	app := buildApp()
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// buildApp binds count and source-free report jobs to one flag parser.
func buildApp() *cli.App {
	// Show both supported jobs with concrete capture and scratch inputs.
	args := &Args{}
	app := cli.NewApp()
	app.Name = "spacewave-recovery"
	app.Usage = "count surviving evidence in an isolated SQLite capture"
	app.UsageText = "spacewave-recovery --source capture.pb --scratch work.sqlite\nspacewave-recovery --scratch work.sqlite --report"
	app.Flags = args.BuildFlags()
	app.Action = args.Run
	return app
}
