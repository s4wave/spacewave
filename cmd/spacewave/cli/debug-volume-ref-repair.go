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
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
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
	// Repair the volume at the argument's path.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	path := c.Args().First()
	le := logrus.NewEntry(logrus.New())
	res, err := repairVolumeRefs(c.Context, le, path, a.spaceID, a.dryRun)
	if err != nil {
		return err
	}

	// Print the repair findings and the volume-wide inventory.
	writeFields(os.Stdout, refRepairFields(path, a.dryRun, res))
	return nil
}

// refRepairFields formats the repair findings in the command's field table.
func refRepairFields(path string, dryRun bool, res refRepairResult) [][2]string {
	// Keep graph repairs separate from the volume-wide inventory.
	fields := [][2]string{
		{"Volume", path},
		{"Dry run", strconv.FormatBool(dryRun)},
		{"Spaces", strconv.Itoa(res.spaces)},
		{"Blocks walked", strconv.FormatUint(res.blocks, 10)},
		{"Blocks absent", strconv.FormatUint(res.absent, 10)},
		{"Blocks undecodable", strconv.FormatUint(res.undecodable, 10)},
		{"Blocks without a child list", strconv.FormatUint(res.lacking, 10)},
		{"Children they name", strconv.FormatUint(res.edges, 10)},
		{"Child lists written", strconv.FormatUint(res.written, 10)},
		{"Plugin prefix groups", strconv.Itoa(len(res.pluginStores))},
		{"Stored blocks without a child list", strconv.FormatUint(res.untracked, 10)},
		{"Their bytes", strconv.FormatUint(res.untrackedBytes, 10)},
	}

	// List private-state prefix groups and a bounded sample of untracked blocks.
	for _, prefix := range slices.Sorted(maps.Keys(res.pluginStores)) {
		unit := " keys"
		if res.pluginStores[prefix] == 1 {
			unit = " key"
		}
		fields = append(fields, [2]string{"Plugin prefix group", prefix + " (" + strconv.FormatUint(res.pluginStores[prefix], 10) + unit + ")"})
	}
	for _, blk := range res.untrackedSample {
		fields = append(fields, [2]string{"Listless sample", blk.ref.MarshalString() + " (" + strconv.FormatUint(blk.size, 10) + " bytes)"})
	}

	// Retain the typed walk's reasons for stopping at an untyped ref.
	for _, reason := range slices.Sorted(maps.Keys(res.untyped)) {
		fields = append(fields, [2]string{"Untyped: " + reason, strconv.FormatUint(res.untyped[reason], 10)})
	}
	return fields
}

// repairVolumeRefs walks the Spaces of the stopped volume at path, all of them
// or spaceID alone, and writes the child lists they lack unless dryRun is set.
func repairVolumeRefs(ctx context.Context, le *logrus.Entry, path, spaceID string, dryRun bool) (refRepairResult, error) {
	// Open the stopped volume.
	vol, err := openStoppedVolume(ctx, le, path)
	if err != nil {
		return refRepairResult{}, err
	}
	defer vol.Close()

	// Read the replay cursors of the selected Spaces.
	cursors, err := readReplayCursors(ctx, volume_s4db.GetDB(vol), spaceID)
	if err != nil {
		return refRepairResult{}, err
	}
	if len(cursors) == 0 {
		return refRepairResult{}, errors.New("no replay cursor found for the selected Spaces")
	}

	// Walk each Space and release the typed snapshots.
	rr := newRefRepair(le, vol)
	defer rr.closeTypes()
	for _, sc := range cursors {
		if err := rr.walkSpace(ctx, sc); err != nil {
			return refRepairResult{}, errors.Wrapf(err, "walk space %s", sc.spaceID)
		}
	}
	rr.closeTypes()

	// Measure the whole volume before applying any of the planned repairs.
	if err := rr.measurePluginStores(ctx, vol); err != nil {
		return refRepairResult{}, errors.Wrap(err, "count plugin object-store prefix groups")
	}
	if err := rr.measureUntracked(ctx, vol); err != nil {
		return refRepairResult{}, errors.Wrap(err, "measure untracked blocks")
	}

	// Write the missing child lists unless this is a dry run.
	if !dryRun {
		if err := rr.apply(ctx); err != nil {
			return refRepairResult{}, err
		}
	}
	return rr.res, nil
}

// newDebugRefRepairCommand returns the offline child list repair command.
func newDebugRefRepairCommand() *cli.Command {
	args := &debugRefRepairArgs{}
	return &cli.Command{
		Name:      "ref-repair",
		Usage:     "write the child lists a stopped, unadopted volume's Spaces lack",
		ArgsUsage: "<volume-file>",
		Description: "Walks the Worlds each Space's replay cursor retains, decoding each block by its type, " +
			"and writes the child list of each block the volume stores without one, so the volume can be adopted. " +
			"It never changes a list it finds, and refuses a volume already adopted. Blocks stored before puts carried their child lists have none. " +
			"Also counts plugin object-store keys by volume prefix and measures all stored blocks without a child list. " +
			"These counts cover the whole volume even with --space, and describe it before repair. The scan does not decode those blocks. " +
			"Run it only on a stopped volume, with --dry-run first.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
