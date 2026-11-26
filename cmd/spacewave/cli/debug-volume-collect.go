//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
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
	}, a.upgrade.BuildFlags()...)
}

// Run collects the stopped volume until a pass leaves no work and prints the
// totals and why an Open generation did not flip.
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

	// Turning frees on recounts the Volume.
	conf := a.upgrade.StoreConfig(&store_kvtx.Config{CollectorFrees: a.frees, CollectorGraceDur: a.grace})
	rep, err := collectVolume(c.Context, logrus.NewEntry(logrus.New()), path, conf)

	// Print the totals, including those a failed pass left.
	writeFields(os.Stdout, rep.fields())
	return err
}

// collectVolume runs collection passes on the stopped volume at path until
// one leaves no work, and reports what they did and what the settle's flip
// checks found after them.
func collectVolume(ctx context.Context, le *logrus.Entry, path string, conf *store_kvtx.Config) (*collectReport, error) {
	// Open the stopped volume with the collector's configuration.
	rep := newCollectReport(path, conf)
	vol, err := openStoppedVolumeStore(ctx, le, path, volumeWritable, conf)
	if err != nil {
		return rep, err
	}
	defer vol.Close()

	// Turning frees on returned the Volume to unadopted for a recount, which
	// the background adoption would run later; count it now, so the passes
	// free rather than find an unadopted Volume and stop.
	start := time.Now()
	defer func() { rep.res.Duration = time.Since(start) }()
	if rep.frees {
		if _, err := vol.Adopt(ctx); err != nil {
			return rep, errors.Wrap(err, "adopt before freeing")
		}
	}

	// Collect, then read what the flip's checks find.
	err = rep.collectToEnd(ctx, vol)
	if statusErr := rep.readStatus(ctx, vol); err == nil {
		err = statusErr
	}
	return rep, err
}

// collectToEnd runs passes on vol until one reports no work left.
func (r *collectReport) collectToEnd(ctx context.Context, vol *volume_s4db.Volume) error {
	for {
		res, err := vol.Collect(ctx)
		r.add(res)
		if err != nil || !res.More {
			return err
		}
	}
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
			"before and after. While the volume's old generation is Open it then prints what the settle's flip still " +
			"waits on: the clause of the free check that fails, a list rewrite owing a recount, the unmarked run, the " +
			"front and the recorded holes with a sample of each, and a ripe block queued to free. Run it only on a " +
			"stopped volume.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
