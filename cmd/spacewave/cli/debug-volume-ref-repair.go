//go:build !js

package spacewave_cli

import (
	"context"
	"maps"
	"os"
	"slices"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/sirupsen/logrus"
)

// debugRefRepairArgs are the arguments of the offline ref graph repair.
type debugRefRepairArgs struct {
	// spaceID limits the repair to one SharedObject. Empty repairs every Space
	// with a replay cursor in the volume.
	spaceID string
	// dryRun counts the missing edges without writing them.
	dryRun bool
}

// BuildFlags returns the ref repair flags.
func (a *debugRefRepairArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "space",
			Usage:       "SharedObject ID of the one Space to repair",
			Destination: &a.spaceID,
		},
		&cli.BoolFlag{
			Name:        "dry-run",
			Usage:       "count the blocks lacking edges without writing them",
			Destination: &a.dryRun,
		},
	}
}

// Run repairs the stopped volume and prints the counts.
func (a *debugRefRepairArgs) Run(c *cli.Context) error {
	// Repair the volume named by the argument.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	path := c.Args().First()
	le := logrus.NewEntry(logrus.New())
	res, err := repairVolumeRefs(c.Context, le, path, a.spaceID, a.dryRun)
	if err != nil {
		return err
	}

	// Print the counts, with each kind of untyped ref.
	fields := [][2]string{
		{"Volume", path},
		{"Dry run", strconv.FormatBool(a.dryRun)},
		{"Spaces", strconv.Itoa(res.spaces)},
		{"Blocks walked", strconv.FormatUint(res.blocks, 10)},
		{"Blocks absent", strconv.FormatUint(res.absent, 10)},
		{"Blocks undecodable", strconv.FormatUint(res.undecodable, 10)},
		{"Blocks lacking edges", strconv.FormatUint(res.lacking, 10)},
		{"Child edges missing", strconv.FormatUint(res.edges, 10)},
		{"Unowned roots", strconv.FormatUint(res.owned, 10)},
		{"Edges added", strconv.FormatUint(res.written, 10)},
	}
	for _, reason := range slices.Sorted(maps.Keys(res.untyped)) {
		fields = append(fields, [2]string{"Untyped: " + reason, strconv.FormatUint(res.untyped[reason], 10)})
	}
	writeFields(os.Stdout, fields)
	return nil
}

// repairVolumeRefs walks the Spaces of the stopped volume at path, all of them
// or spaceID alone, and adds the edges they lack unless dryRun is set.
func repairVolumeRefs(ctx context.Context, le *logrus.Entry, path, spaceID string, dryRun bool) (refRepairResult, error) {
	// Hold the stopped volume exclusively.
	if err := requireVolumeStopped(path); err != nil {
		return refRepairResult{}, err
	}
	vol, err := volume_bolt.NewBolt(ctx, le, &volume_bolt.Config{
		Path:          path,
		NoGenerateKey: true,
		NoWriteKey:    true,
		Exclusive:     true,
	})
	if err != nil {
		return refRepairResult{}, errors.Wrap(err, "open volume")
	}
	defer vol.Close()

	// Read the replay cursors of the selected Spaces.
	cursors, err := readReplayCursors(volume_bolt.GetBoltDB(vol), spaceID)
	if err != nil {
		return refRepairResult{}, err
	}
	if len(cursors) == 0 {
		return refRepairResult{}, errors.New("no replay cursor found for the selected Spaces")
	}

	// Walk each Space, then write the missing edges unless this is a dry run.
	rr := newRefRepair(le, vol, vol.GetRefGraph())
	defer rr.closeTypes()
	for _, sc := range cursors {
		if err := rr.walkSpace(ctx, sc); err != nil {
			return refRepairResult{}, errors.Wrapf(err, "walk space %s", sc.spaceID)
		}
	}
	rr.closeTypes()
	if !dryRun {
		if err := rr.apply(ctx); err != nil {
			return refRepairResult{}, err
		}
	}
	return rr.res, nil
}

// newDebugRefRepairCommand returns the offline ref graph repair command.
func newDebugRefRepairCommand() *cli.Command {
	args := &debugRefRepairArgs{}
	return &cli.Command{
		Name:      "ref-repair",
		Usage:     "add the ref graph edges a stopped volume's Spaces lack",
		ArgsUsage: "<volume-file>",
		Description: "Walks the Worlds each Space's replay cursor retains, decoding each block by its type, " +
			"and adds every edge from a block to its refs that the ref graph lacks, plus a bucket edge to each root " +
			"nothing holds. It never removes an edge. Blocks written while GC tracking was off carry no edges. " +
			"Run it only on a stopped volume, with --dry-run first.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
