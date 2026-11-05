//go:build !js

package main

import (
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/cmd/spacewave-store-bench/storebench"
)

// RunArgs drive a synthetic server load on the store and report its speed.
type RunArgs struct {
	StoreArgs
	// Load shapes the load.
	Load storebench.LoadConfig
	// ReportPath is where the JSON report is written; empty prints it.
	ReportPath string
}

// BuildFlags binds the store and load flags. The load defaults follow the
// server load of 2026-10-26: about 6,900 views and a peak of 37.6 writes/s.
func (a *RunArgs) BuildFlags() []cli.Flag {
	return append(a.StoreArgs.BuildFlags(),
		&cli.StringFlag{Name: "label", Usage: "names the run in the report", Destination: &a.Load.Label},
		&cli.IntFlag{Name: "watchers", Usage: "view watchers", Destination: &a.Load.Watchers, Value: 6900},
		&cli.IntFlag{Name: "view-reads", Usage: "store objects each view reads", Destination: &a.Load.ViewReads, Value: 4},
		&cli.Float64Flag{Name: "hot-view-fraction", Usage: "share of views that read a written object", Destination: &a.Load.HotViewFraction, Value: 0.1},
		&cli.DurationFlag{Name: "view-interval", Usage: "least time between two runs of one view", Destination: &a.Load.ViewInterval, Value: time.Second},
		&cli.IntFlag{Name: "sample-keys", Usage: "store object keys sampled for reads", Destination: &a.Load.SampleKeys, Value: 20000},
		&cli.Float64Flag{Name: "write-rate", Usage: "World writes started per second", Destination: &a.Load.WriteRate, Value: 37.6},
		&cli.IntFlag{Name: "writers", Usage: "writes that may run at once", Destination: &a.Load.Writers, Value: 8},
		&cli.IntFlag{Name: "hot-keys", Usage: "objects the writes update", Destination: &a.Load.HotKeys, Value: 64},
		&cli.IntFlag{Name: "body-bytes", Usage: "object body bytes per write", Destination: &a.Load.BodyBytes, Value: 3200},
		&cli.IntFlag{Name: "write-reads", Usage: "store objects each write reads", Destination: &a.Load.WriteReads, Value: 4},
		&cli.DurationFlag{Name: "warmup", Usage: "load run before measuring", Destination: &a.Load.Warmup, Value: 30 * time.Second},
		&cli.DurationFlag{Name: "duration", Usage: "measured load", Destination: &a.Load.Duration, Value: 2 * time.Minute},
		&cli.Uint64Flag{Name: "seed", Usage: "seeds the key sampling", Destination: &a.Load.Seed, Value: 1},
		&cli.StringFlag{Name: "cpu-profile", Usage: "write the measured CPU profile here", Destination: &a.Load.ProfilePath},
		&cli.StringFlag{Name: "alloc-profile", Usage: "write heap allocation profiles of the window's start and end here", Destination: &a.Load.AllocProfilePath},
		&cli.StringFlag{Name: "stall-dump-dir", Usage: "write an execution trace of the first stalled write here", Destination: &a.Load.StallDumpDir},
		&cli.DurationFlag{Name: "stall-threshold", Usage: "how long a write runs before it counts as stalled", Destination: &a.Load.StallThreshold, Value: 500 * time.Millisecond},
		&cli.BoolFlag{Name: "stall-stacks", Usage: "also dump goroutine stacks at each stall, which stops the world", Destination: &a.Load.StallStacks},
		&cli.BoolFlag{Name: "gc", Usage: "run collection passes back to back through the measured window", Destination: &a.Load.GCMark},
		&cli.StringFlag{Name: "report", Usage: "write the JSON report here instead of printing it", Destination: &a.ReportPath},
	)
}

// Run opens the store, drives the load and writes the report.
func (a *RunArgs) Run(c *cli.Context) error {
	// Open the store's World.
	ctx, le := c.Context, a.logger()
	st, err := storebench.OpenStore(ctx, le, &a.Store, false)
	if err != nil {
		return err
	}
	defer st.Close()

	// Drive the load and encode its report.
	report, err := storebench.RunLoad(ctx, le, st, &a.Load)
	if err != nil {
		return err
	}
	data, err := report.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "encode report")
	}
	data = append(data, '\n')

	// Print the report or write it to its file.
	if a.ReportPath == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(a.ReportPath, data, 0o644)
}
