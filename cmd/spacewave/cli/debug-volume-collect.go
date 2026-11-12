//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/sirupsen/logrus"
)

// debugVolumeCollectArgs are the arguments of the offline collection.
type debugVolumeCollectArgs struct {
	// frees turns the collector's frees on.
	frees bool
	// grace is the collector's grace period, empty for its default.
	grace string
}

// BuildFlags returns the collect flags.
func (a *debugVolumeCollectArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:        "frees",
			Usage:       "free what no named root reaches; off, the passes only net the journal",
			Destination: &a.frees,
		},
		&cli.StringFlag{
			Name:        "grace",
			Usage:       "how old a journal record must be before a pass nets it, such as 15m; empty uses the collector's default",
			Destination: &a.grace,
		},
	}
}

// Run collects the stopped volume until a pass leaves no work and prints the
// totals.
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
	conf := &store_kvtx.Config{CollectorFrees: a.frees, CollectorGraceDur: a.grace}
	res, passes, err := collectVolume(c.Context, logrus.NewEntry(logrus.New()), path, conf)

	// Print the totals, including those a failed pass left.
	writeFields(os.Stdout, collectFields(path, a.frees, passes, res))
	return err
}

// collectVolume runs collection passes on the stopped volume at path until one
// leaves no work, and returns their sum and the number of passes.
func collectVolume(ctx context.Context, le *logrus.Entry, path string, conf *store_kvtx.Config) (block_gc.CollectResult, int, error) {
	// Open the stopped volume with the collector's configuration.
	var sum block_gc.CollectResult
	vol, err := openStoppedVolumeStore(ctx, le, path, conf)
	if err != nil {
		return sum, 0, err
	}
	defer vol.Close()

	// Collect until a pass reports no work left.
	start := time.Now()
	for passes := 1; ; passes++ {
		res, err := vol.Collect(ctx)
		sum.Netted += res.Netted
		sum.Counted += res.Counted
		sum.Queued += res.Queued
		sum.Freed += res.Freed
		sum.Kept += res.Kept
		sum.Parked += res.Parked
		sum.Swept += res.Swept
		if res.Stall != block_gc.StallNone {
			sum.Stall = res.Stall
		}
		sum.Leaked += res.Leaked
		sum.Stranded += res.Stranded
		sum.Underflows += res.Underflows
		sum.Lapsed += res.Lapsed
		sum.Duration = time.Since(start)
		if err != nil || !res.More {
			return sum, passes, err
		}
	}
}

// collectFields formats the collection totals in the command's field table.
func collectFields(path string, frees bool, passes int, res block_gc.CollectResult) [][2]string {
	return [][2]string{
		{"Volume", path},
		{"Frees", strconv.FormatBool(frees)},
		{"Passes", strconv.Itoa(passes)},
		{"Netted records", strconv.FormatUint(res.Netted, 10)},
		{"Counted blocks", strconv.FormatUint(res.Counted, 10)},
		{"Queued to free", strconv.FormatUint(res.Queued, 10)},
		{"Freed", strconv.FormatUint(res.Freed, 10)},
		{"Kept", strconv.FormatUint(res.Kept, 10)},
		{"Parked older blocks", strconv.FormatUint(res.Parked, 10)},
		{"Swept older blocks", strconv.FormatUint(res.Swept, 10)},
		{"Stall", res.Stall.String()},
		{"Leaked", strconv.FormatUint(res.Leaked, 10)},
		{"Stranded releases", strconv.FormatUint(res.Stranded, 10)},
		{"Underflows refused", strconv.FormatUint(res.Underflows, 10)},
		{"Lapsed leases", strconv.FormatUint(res.Lapsed, 10)},
		{"Duration", res.Duration.String()},
	}
}

// newDebugVolumeCollectCommand returns the offline collection command.
func newDebugVolumeCollectCommand() *cli.Command {
	args := &debugVolumeCollectArgs{}
	return &cli.Command{
		Name:      "volume-collect",
		Usage:     "run collection passes on a stopped volume until one leaves no work",
		ArgsUsage: "<volume-file>",
		Description: "Runs the volume's collector as the daemon's sweep does, pass after pass until one reports no work " +
			"left, and prints the totals. Without --frees the passes release lapsed leases and net the journal and free " +
			"nothing. With --frees an adopted volume frees what lost its last holder; this deletes blocks, so run it on a " +
			"copy for a rehearsal, and compare a volume-audit before and after. Run it only on a stopped volume.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
