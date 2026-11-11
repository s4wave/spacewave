//go:build !js && !wasip1

package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	_ "net/http/pprof" // #nosec G108 -- served only on the --pprof address.
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"golang.org/x/sync/errgroup"
)

// runArgs regenerate the World, or switch it back or release it, while a
// writer commits results at a fixed rate.
type runArgs struct {
	// Path is the database file.
	Path string
	// Config is the deployment profile the World opens as.
	Config string
	// SpillDir is the directory of the regeneration's sort runs.
	SpillDir string
	// Budget is the sort's memory in bytes.
	Budget uint64
	// SwapChanges is the most quad changes the swap applies in the turn.
	SwapChanges uint64
	// Floor is the least free disk the build leaves beside the database.
	Floor uint64
	// Rate is the writer's commits a second.
	Rate float64
	// Batch is the results in each writer commit.
	Batch int
	// Warmup is how long the writer commits before the first step, so the
	// latency file holds a baseline.
	Warmup time.Duration
	// Until is the stage the steps stop at: switched or done.
	Until string
	// Back switches back instead of stepping forward.
	Back bool
	// Release releases the regeneration's roots instead of stepping.
	Release bool
	// Hold is how long the writer keeps committing after the last step.
	Hold time.Duration
	// Pprof is the address serving the Go profiles; empty serves none.
	Pprof string
}

// BuildFlags binds the flags.
func (a *runArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "path", Usage: "database file", Destination: &a.Path, Required: true},
		configFlag(&a.Config),
		&cli.StringFlag{Name: "spill-dir", Usage: "directory for the sort's runs, the database's by default", Destination: &a.SpillDir},
		&cli.Uint64Flag{Name: "budget", Usage: "sort memory in bytes", Destination: &a.Budget, Value: 256 << 20},
		&cli.Uint64Flag{Name: "floor", Usage: "least free disk in bytes the build leaves", Destination: &a.Floor, Value: 4 << 30},
		&cli.Uint64Flag{Name: "swap-changes", Usage: "most quad changes the swap applies in the writer turn", Destination: &a.SwapChanges, Value: 4096},
		&cli.Float64Flag{Name: "rate", Usage: "writer commits a second", Destination: &a.Rate, Value: 0.2},
		&cli.IntFlag{Name: "batch", Usage: "results per writer commit", Destination: &a.Batch, Value: 32},
		&cli.DurationFlag{Name: "warmup", Usage: "writer time before the first step", Destination: &a.Warmup},
		&cli.StringFlag{Name: "until", Usage: "stage to stop at: switched or done", Destination: &a.Until, Value: "done"},
		&cli.BoolFlag{Name: "back", Usage: "switch back instead of stepping forward", Destination: &a.Back},
		&cli.BoolFlag{Name: "release", Usage: "release the regeneration's roots", Destination: &a.Release},
		&cli.DurationFlag{Name: "hold", Usage: "writer time after the last step", Destination: &a.Hold},
		&cli.StringFlag{Name: "pprof", Usage: "serve the Go profiles on this address", Destination: &a.Pprof},
	}
}

// Run regenerates the World beside the writer.
func (a *runArgs) Run(c *cli.Context) error {
	// Serve the profiles.
	ctx := c.Context
	if a.Pprof != "" {
		srv := &http.Server{Addr: a.Pprof, ReadHeaderTimeout: time.Second}
		go func() { _ = srv.ListenAndServe() }()
		defer srv.Close()
	}

	// Open the World and report the stage it starts from.
	w, err := openBenchWorld(ctx, a.Path, a.Config)
	if err != nil {
		return err
	}
	defer w.Close()
	var stage atomic.Value
	st, err := w.stage(ctx)
	if err != nil {
		return err
	}
	stage.Store(st.String())
	logf("stage %v", st)

	// Commit beside the steps until they and the hold end.
	writeCtx, stopWriter := context.WithCancel(ctx)
	defer stopWriter()
	var g errgroup.Group
	g.Go(func() error { return a.write(writeCtx, w, &stage) })
	err = a.steps(ctx, w, &stage)
	if err == nil {
		err = sleep(ctx, a.Hold)
	}

	// Stop the writer and report the first error.
	stopWriter()
	if werr := g.Wait(); err == nil {
		err = werr
	}
	return err
}

// regenConfig returns the engine's regeneration config for the flags.
func (a *runArgs) regenConfig() *world_block_engine.RegenConfig {
	// Choose the target.
	target := world_block_engine.RegenTarget_REGEN_TARGET_DONE
	switch {
	case a.Release:
		target = world_block_engine.RegenTarget_REGEN_TARGET_RELEASED
	case a.Back:
		target = world_block_engine.RegenTarget_REGEN_TARGET_SWITCHED_BACK
	case a.Until == world_block.RegenSwitched.String():
		target = world_block_engine.RegenTarget_REGEN_TARGET_SWITCHED
	}

	// Build beside the database unless told otherwise, swapping at most what
	// the config holds.
	swap := uint32(math.MaxUint32)
	if a.SwapChanges < math.MaxUint32 {
		swap = uint32(a.SwapChanges)
	}
	spill := a.SpillDir
	if spill == "" {
		spill = filepath.Dir(a.Path)
	}
	return &world_block_engine.RegenConfig{
		Target:      target,
		SpillDir:    spill,
		BudgetBytes: a.Budget,
		SwapChanges: swap,
		FloorDir:    filepath.Dir(a.Path),
		FloorBytes:  a.Floor,
	}
}

// steps waits out the warmup, then restarts the engine controller with the
// regeneration target and follows its log until it reaches the target,
// storing each stage reached in stage.
func (a *runArgs) steps(ctx context.Context, w *benchWorld, stage *atomic.Value) error {
	// Let the writer set a baseline.
	if err := sleep(ctx, a.Warmup); err != nil {
		return err
	}

	// Restart with the target, as an operator does.
	start := time.Now()
	if err := w.restart(ctx, a.regenConfig()); err != nil {
		return err
	}
	logf("restarted with the target after %v", time.Since(start).Round(time.Millisecond))

	// Follow the steps until the target or a stop.
	for {
		var ev regenEvent
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev = <-w.hook.regen:
		}
		switch ev.msg {
		case "world regeneration step":
			stage.Store(ev.stage)
			logf("stage %v after %v", ev.stage, ev.took)
		case "world regeneration reached its target":
			stage.Store(ev.stage)
			logf("reached %v after %v", ev.stage, time.Since(start).Round(time.Millisecond))
			return nil
		case "world regeneration stopped":
			return errors.Wrap(ev.err, "regeneration stopped")
		}
	}
}

// write commits Batch new results Rate times a second until ctx ends,
// appending each acknowledged commit's end and latency to the files beside
// the database. It resumes past the seed and the last acknowledged commit.
func (a *runArgs) write(ctx context.Context, w *benchWorld, stage *atomic.Value) error {
	// Find the next result.
	seeded, err := lastInt(seededPath(a.Path))
	if err != nil {
		return err
	}
	acked, err := lastInt(ackedPath(a.Path))
	if err != nil {
		return err
	}
	next := max(seeded, acked)

	// Commit on each tick, timing the commit from asking for the turn.
	tick := time.NewTicker(time.Duration(float64(time.Second) / a.Rate))
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		start := time.Now()
		err := commitTx(ctx, w, func(ws world.WorldState) error {
			return writeResults(ctx, ws, next, next+a.Batch)
		})
		took := time.Since(start)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return errors.Wrapf(err, "commit results from %d", next)
		}
		next += a.Batch
		if err := appendLine(ackedPath(a.Path), strconv.Itoa(next)); err != nil {
			return err
		}
		line := fmt.Sprintf("%d %d %s", took.Nanoseconds(), start.UnixNano(), stage.Load())
		if err := appendLine(latencyPath(a.Path), line); err != nil {
			return err
		}
	}
}

// sleep waits d or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// logf prints a timestamped progress line on stderr.
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s "+format+"\n", append([]any{time.Now().Format(time.TimeOnly)}, args...)...)
}

// configFlag binds the deployment profile flag to dst.
func configFlag(dst *string) cli.Flag {
	return &cli.StringFlag{Name: "config", Usage: "deployment the World opens as: " + profileNames, Destination: dst, Value: "spacewave"}
}
