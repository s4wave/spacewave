//go:build !js

package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/aperturerobotics/cli"
)

func main() {
	// Register the one command.
	var args BenchArgs
	app := cli.NewApp()
	app.Name = "spacewave-s4db-bench"
	app.HideVersion = true
	app.Usage = "commit uniform keys into one s4db file and report commit latency while flushes and merges run"
	app.Flags = args.BuildFlags()
	app.Action = args.Run

	// Run it until it ends or the process is interrupted.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := app.RunContext(ctx, os.Args); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
