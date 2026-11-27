//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// debugVolumeCollectArgs are the arguments of the offline collection.
type debugVolumeCollectArgs struct {
	// frees turns the collector's frees on.
	frees bool
	// grace is the collector's grace period, empty for its default.
	grace string
	// load is the writer run beside the passes.
	load volumeLoadOpts
	// cpuProfile is the file that receives a CPU profile of the run, empty
	// for none.
	cpuProfile string
	// upgrade decides whether the open upgrades the volume's format.
	upgrade *FormatUpgrade
}

// BuildFlags returns the collect flags.
func (a *debugVolumeCollectArgs) BuildFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.BoolFlag{
			Name:        "frees",
			Usage:       "free what no named root reaches; off, the passes only net the logs",
			Destination: &a.frees,
		},
		&cli.StringFlag{
			Name:        "grace",
			Usage:       "how long a block waits at zero before a pass frees it, such as 15m; empty uses the collector's default",
			Destination: &a.grace,
		},
		&cli.Float64Flag{
			Name:        "load-rate",
			Usage:       "commits per second of the writer --load-for runs",
			Value:       12,
			Destination: &a.load.rate,
		},
		&cli.DurationFlag{
			Name:        "load-for",
			Usage:       "run a writer beside the passes for this long, such as 10m, and print its commit latencies",
			Destination: &a.load.window,
		},
		&cli.StringFlag{
			Name:        "cpu-profile",
			Usage:       "write a Go CPU profile of the whole run, the recount included, to this file",
			Destination: &a.cpuProfile,
		},
	}, a.upgrade.BuildFlags()...)
}

// Run collects the stopped volume until a pass leaves no work, or under a
// writer until its window ends, and prints the totals and why an Open
// generation did not flip.
func (a *debugVolumeCollectArgs) Run(c *cli.Context) error {
	// Collect the volume at the argument's path.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	if a.grace != "" {
		if _, err := time.ParseDuration(a.grace); err != nil {
			return errors.Wrap(err, "grace")
		}
	}
	path := c.Args().First()

	// Profile from before the open, so the profile covers the recount.
	stopProfile, err := startCPUProfile(a.cpuProfile)
	if err != nil {
		return err
	}

	// Turning frees on recounts the Volume. An interrupt or a TERM ends the
	// run at its next commit, so a stopped run still prints its totals.
	ctx, stop := signal.NotifyContext(c.Context, os.Interrupt, syscall.SIGTERM)
	defer stop()
	conf := a.upgrade.StoreConfig(&store_kvtx.Config{CollectorFrees: a.frees, CollectorGraceDur: a.grace})
	rep, err := collectVolume(ctx, logrus.NewEntry(logrus.New()), path, conf, a.load)
	err = stderrors.Join(err, stopProfile())

	// Print the totals, including those a failed or stopped pass left.
	writeFields(os.Stdout, rep.fields())
	return err
}

// collectVolume runs collection passes on the stopped volume at path until
// one leaves no work or, when load has a window, beside a writer until the
// window ends, and reports what they did and what the settle's flip checks
// found after them.
func collectVolume(ctx context.Context, le *logrus.Entry, path string, conf *store_kvtx.Config, load volumeLoadOpts) (*collectReport, error) {
	// Open the stopped volume with the collector's configuration.
	rep := newCollectReport(path, conf)
	vol, err := openStoppedVolumeStore(ctx, le, path, volumeWritable, conf)
	if err != nil {
		return rep, err
	}
	defer vol.Close()

	// Turning frees on returned the Volume to unadopted for a recount,
	// which the background adoption would run later. Count the Volume now,
	// so the passes free rather than find an unadopted Volume and stop.
	start := time.Now()
	defer func() { rep.res.Duration = time.Since(start) }()
	if rep.frees {
		if err := adoptLogged(ctx, le, vol); err != nil {
			return rep, errors.Wrap(err, "adopt before freeing")
		}
	}

	// Log the passes' progress from here.
	rep.le, rep.logged = le, time.Now()

	// Collect, under the writer when it has a window, then read what the
	// flip's checks find.
	if load.window > 0 {
		err = rep.collectUnderLoad(ctx, vol, load)
	} else {
		err = rep.collectToEnd(ctx, vol)
	}
	if statusErr := rep.readStatus(ctx, vol); err == nil {
		err = statusErr
	}
	return rep, err
}

// adoptLogged adopts vol and logs what the count found and how long it took. A
// recount of a large Volume runs for hours and the command's report prints only
// at exit, so this log line is what says the passes have begun.
func adoptLogged(ctx context.Context, le *logrus.Entry, vol *volume_s4db.Volume) error {
	// Adopt. A Volume already adopted counted nothing.
	start := time.Now()
	res, err := vol.Adopt(ctx)
	if err != nil || res.Already {
		return err
	}

	// Log the count.
	le.WithFields(logrus.Fields{
		"blocks":   res.Blocks,
		"counted":  res.Counted,
		"queued":   res.Queued,
		"duration": time.Since(start).Round(time.Second).String(),
	}).Info("counted the volume's holders")
	return nil
}

// pass runs one collection pass on vol, adds its accounting and logs the
// totals so far.
func (r *collectReport) pass(ctx context.Context, vol *volume_s4db.Volume) (block_gc.CollectResult, error) {
	res, err := vol.Collect(ctx)
	r.add(res)
	r.progress()
	return res, err
}

// collectToEnd runs passes on vol until one reports no work left.
func (r *collectReport) collectToEnd(ctx context.Context, vol *volume_s4db.Volume) error {
	for {
		res, err := r.pass(ctx, vol)
		if err != nil || !res.More {
			return err
		}
	}
}

// collectUnderLoad runs a writer on vol as load sets, and passes beside the
// writer in the same process until the writer's window ends. Each pass after
// one that left no work waits for the writer's next commit. The function then
// releases the writer's scratch root.
func (r *collectReport) collectUnderLoad(ctx context.Context, vol *volume_s4db.Volume, load volumeLoadOpts) error {
	// Start the writer, which ends the window early if a commit fails.
	w, err := newVolumeLoad(ctx, vol, load.rate)
	if err != nil {
		return err
	}
	r.load = w
	windowCtx, cancel := context.WithTimeout(ctx, load.window)
	defer cancel()
	kick := make(chan struct{}, 1)
	loadErr := make(chan error, 1)
	go func() {
		err := w.run(windowCtx, kick)
		if err != nil {
			cancel()
		}
		loadErr <- err
	}()

	// Collect until the window ends.
	var passErr error
	for passErr == nil && windowCtx.Err() == nil {
		var res block_gc.CollectResult
		res, passErr = r.pass(ctx, vol)
		if passErr == nil && !res.More {
			select {
			case <-windowCtx.Done():
			case <-kick:
			}
		}
	}

	// Stop the writer, then release its scratch root.
	cancel()
	err = <-loadErr
	if err == nil {
		err = passErr
	}
	if releaseErr := w.release(ctx); err == nil {
		err = releaseErr
	}
	return err
}

// newDebugVolumeCollectCommand returns the offline collection command.
func newDebugVolumeCollectCommand(upgrade *FormatUpgrade) *cli.Command {
	args := &debugVolumeCollectArgs{upgrade: upgrade}
	return &cli.Command{
		Name:      "volume-collect",
		Usage:     "run collection passes on a stopped volume until one leaves no work",
		ArgsUsage: "<volume-file>",
		Description: "Runs the volume's collector as the daemon's sweep does, pass after pass until one reports no work " +
			"left, and prints the totals. Without --frees the passes release lapsed leases and net the logs and free " +
			"nothing. With --frees the volume is first counted again, as turning frees on requires, and then frees what " +
			"lost its last holder; this deletes blocks, so run it on a copy for a rehearsal, and compare a volume-audit " +
			"before and after. With --load-for, a writer in the same process commits a tree of eight 3,200-byte " +
			"leaves and a parent under a scratch root --load-rate times a second while the passes run, prints each " +
			"commit's latency distribution, and releases the root at the end. While the volume's old generation is Open it then prints what the settle's flip still " +
			"waits on: the clause of the free check that fails, a list rewrite owing a recount, the unmarked run, the " +
			"front and the recorded holes with a sample of each, and a ripe block queued to free. Run it only on a " +
			"stopped volume.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
