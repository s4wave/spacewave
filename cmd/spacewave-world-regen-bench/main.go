//go:build !js && !wasip1

// Command spacewave-world-regen-bench rehearses a World's regeneration at
// production size on one s4db file: it seeds a World shaped like Hyperline's
// course results in the old format, then regenerates it while a writer
// commits, through any number of kills, and checks the result.
//
// Every acknowledged commit and its latency go to append-only files beside
// the database, so a run killed at any point leaves what check needs to prove
// the head holds exactly the seed and every acknowledged commit.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/aperturerobotics/cli"
)

// main runs the command the arguments name.
func main() {
	// Describe the commands.
	seed, run, check, report := &seedArgs{}, &runArgs{}, &checkArgs{}, &reportArgs{}
	app := cli.NewApp()
	app.Name = "spacewave-world-regen-bench"
	app.Usage = "rehearse a World's regeneration at production size while a writer commits"
	app.Commands = []*cli.Command{
		{Name: "seed", Usage: "seed the old-format World, resuming a stopped seed", Flags: seed.BuildFlags(), Action: seed.Run},
		{Name: "run", Usage: "regenerate while the writer commits, stopping at --until", Flags: run.BuildFlags(), Action: run.Run},
		{Name: "check", Usage: "prove the head holds the seed and every acknowledged commit", Flags: check.BuildFlags(), Action: check.Run},
		{Name: "report", Usage: "summarize the commit latencies by regeneration stage", Flags: report.BuildFlags(), Action: report.Run},
	}

	// Run the one named until it ends or the process is interrupted.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := app.RunContext(ctx, os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
