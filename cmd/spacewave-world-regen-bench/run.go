//go:build !js && !wasip1

package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // #nosec G108 -- served only on the --pprof address.
	"os"
	"path/filepath"
	"runtime/metrics"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"golang.org/x/sync/errgroup"
)

// runArgs regenerate the World, or switch it back or release it, while a
// writer commits results at a fixed rate.
type runArgs struct {
	// Path is the database file.
	Path string
	// SpillDir is the directory of the regeneration's sort runs.
	SpillDir string
	// Budget is the sort's memory in bytes.
	Budget int
	// SwapChanges is the most quad changes the swap applies in the turn.
	SwapChanges int
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
		&cli.StringFlag{Name: "spill-dir", Usage: "directory for the sort's runs, the database's by default", Destination: &a.SpillDir},
		&cli.IntFlag{Name: "budget", Usage: "sort memory in bytes", Destination: &a.Budget, Value: 256 << 20},
		&cli.Uint64Flag{Name: "floor", Usage: "least free disk in bytes the build leaves", Destination: &a.Floor, Value: 4 << 30},
		&cli.IntFlag{Name: "swap-changes", Usage: "most quad changes the swap applies in the writer turn", Destination: &a.SwapChanges, Value: 4096},
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

// Run steps the regeneration beside the writer.
func (a *runArgs) Run(c *cli.Context) error {
	// Serve the profiles.
	ctx := c.Context
	if a.Pprof != "" {
		srv := &http.Server{Addr: a.Pprof, ReadHeaderTimeout: time.Second}
		go func() { _ = srv.ListenAndServe() }()
		defer srv.Close()
	}

	// Open the World and its Regenerator.
	w, err := openBenchWorld(ctx, a.Path)
	if err != nil {
		return err
	}
	defer w.Close()
	cfg := world_block.RegenConfig{
		SpillDir:    a.SpillDir,
		Budget:      a.Budget,
		SwapChanges: a.SwapChanges,
		FloorDir:    filepath.Dir(a.Path),
		FloorBytes:  a.Floor,
	}
	if cfg.SpillDir == "" {
		cfg.SpillDir = filepath.Dir(a.Path)
	}
	r := world_block.NewRegenerator(w.authority, cfg)

	// Report the stage it starts from.
	var stage atomic.Value
	st, err := r.Stage(ctx)
	if err != nil {
		return err
	}
	stage.Store(st.String())
	logf("stage %v", st)

	// Commit beside the steps until they and the hold end, logging memory.
	writeCtx, stopWriter := context.WithCancel(ctx)
	defer stopWriter()
	var g errgroup.Group
	g.Go(func() error { return a.write(writeCtx, w, &stage) })
	g.Go(func() error { logMemory(writeCtx, memoryLogInterval); return nil })
	err = a.steps(ctx, r, &stage)
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

// steps waits out the warmup, then takes the steps the flags ask for, storing
// each stage reached in stage.
func (a *runArgs) steps(ctx context.Context, r *world_block.Regenerator, stage *atomic.Value) error {
	// Let the writer set a baseline.
	if err := sleep(ctx, a.Warmup); err != nil {
		return err
	}

	// Release or switch back in one call.
	start := time.Now()
	if a.Release || a.Back {
		call := r.SwitchBack
		if a.Release {
			call = r.Release
		}
		if err := call(ctx); err != nil {
			return err
		}
		st, err := r.Stage(ctx)
		stage.Store(st.String())
		logf("stage %v after %v", st, time.Since(start).Round(time.Millisecond))
		return err
	}

	// Step forward until the stage asked for.
	until := world_block.RegenDone
	if a.Until == world_block.RegenSwitched.String() {
		until = world_block.RegenSwitched
	}
	return r.Run(ctx, until, func(st world_block.RegenStage, took time.Duration) {
		stage.Store(st.String())
		logf("stage %v after %v", st, took.Round(time.Millisecond))
	})
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
// memoryLogInterval is how often a run logs its memory.
const memoryLogInterval = 5 * time.Second

// logMemory logs, every interval until ctx ends, the decoded-block pool's
// retained cost against its budget and the Go heap's live object bytes, so a
// run log shows how much of the heap the pool holds. The heap figure comes
// from runtime/metrics, which reads it without stopping the world.
func logMemory(ctx context.Context, interval time.Duration) {
	// Read the process-wide pool through a scope of its own.
	pool := block.NewDecodedBlockCache()
	defer pool.Close()
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}

	// Log one line each interval.
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		st := pool.Snapshot()
		metrics.Read(sample)
		heap := int64(sample[0].Value.Uint64()) //nolint:gosec // the live heap is far below 2^63 bytes.
		logf("memory heap %d pool %d of %d (%d entries) beyond %d", heap, st.RetainedCost, st.MaxCost, st.Entries, heap-st.RetainedCost)
	}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s "+format+"\n", append([]any{time.Now().Format(time.TimeOnly)}, args...)...)
}
