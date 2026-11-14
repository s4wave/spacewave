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
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world/refwalk"
	"github.com/sirupsen/logrus"
)

// debugRefRepairArgs are the arguments of the offline ref graph repair.
type debugRefRepairArgs struct {
	// spaceID limits the repair to one SharedObject. Empty repairs every Space
	// with a replay cursor in the volume and every named root it can type.
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
	res, err := repairVolumeRefs(c.Context, le, path, a.spaceID, a.dryRun, space_world_optypes.LookupBlockType)
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
		{"Named roots", strconv.FormatUint(res.roots, 10)},
		{"Named roots in walked Spaces' buckets", strconv.FormatUint(res.spaceRoots, 10)},
		{"Named roots walked by type", strconv.FormatUint(res.typedRoots, 10)},
		{"Blocks walked", strconv.FormatUint(res.blocks, 10)},
		{"Blocks absent", strconv.FormatUint(res.absent, 10)},
		{"Blocks undecodable", strconv.FormatUint(res.undecodable, 10)},
		{"Blocks without a child list", strconv.FormatUint(res.lacking, 10)},
		{"Children they name", strconv.FormatUint(res.edges, 10)},
		{"Child lists verified", strconv.FormatUint(res.rebuild.Verified, 10)},
		{"Child lists written", strconv.FormatUint(res.rebuild.Rewritten, 10)},
		{"Decoded lists refused for dropping a stored child", strconv.FormatUint(res.rebuild.Dropped, 10)},
		{"Reached lists still unverified", strconv.FormatUint(res.rebuild.Unverified, 10)},
		{"Lists verified", strconv.FormatBool(res.verified || res.rebuild.Cleared)},
		{"Returned to unadopted for a recount", strconv.FormatBool(res.rebuild.Unadopted)},
		{"Plugin prefix groups", strconv.Itoa(len(res.pluginStores))},
		{"Stored blocks without a child list", strconv.FormatUint(res.untracked, 10)},
		{"Their bytes", strconv.FormatUint(res.untrackedBytes, 10)},
		{"Holders of listless blocks after the run", strconv.FormatUint(res.holders.total(), 10)},
		{"Held by named roots", strconv.FormatUint(res.holders.roots, 10)},
		{"Held by reached child lists", strconv.FormatUint(res.holders.reached, 10)},
		{"Held by leaked child lists", strconv.FormatUint(res.holders.leaked, 10)},
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

	// Name the reached lists the walk could not verify, which keep the
	// collector from freeing older blocks, and the refused reports.
	for _, ref := range res.rebuild.UnverifiedSample {
		fields = append(fields, [2]string{"Unverified sample", ref.MarshalString()})
	}
	for _, ref := range res.rebuild.DroppedSample {
		fields = append(fields, [2]string{"Refused sample", ref.MarshalString()})
	}

	// Name the roots not walked by kind, the blocks left listless that an
	// adoption counts as unknown, and the leaked listers naming them.
	for _, kind := range slices.Sorted(maps.Keys(res.unwalkedRoots)) {
		roots := res.unwalkedRoots[kind]
		fields = append(fields, [2]string{"Named roots not walked: " + kind, strconv.FormatUint(roots.count, 10) + " (" + roots.sample + ")"})
	}
	for _, ref := range res.holders.heldSample {
		fields = append(fields, [2]string{"Held listless sample", ref.MarshalString()})
	}
	for _, ref := range res.holders.leakedSample {
		fields = append(fields, [2]string{"Leaked lister sample", ref.MarshalString()})
	}

	// Retain the walk's reasons for stopping at an untyped ref, with a sample
	// ref of each.
	for _, reason := range slices.Sorted(maps.Keys(res.untyped)) {
		untyped := res.untyped[reason]
		fields = append(fields, [2]string{"Untyped: " + reason, strconv.FormatUint(untyped.Count, 10) + " (" + untyped.Sample.MarshalString() + ")"})
	}
	return fields
}

// repairVolumeRefs walks the Spaces of the stopped volume at path, all of them
// with every named root it can type, or spaceID alone, typing objects' roots
// with lookup, and unless dryRun is set rebuilds the child lists they reach
// from the decoded blocks, clearing the volume's unverified lists mark when
// every list a root reaches is verified. It then splits the holders an
// adoption would refuse, measured after a real run and predicted on a dry run. Once the mark is clear the
// rebuild refuses the volume and the run only walks it, so a list found short
// after that is not repaired here: re-verifying a verified volume needs a step
// that sets the mark again, and none exists yet.
func repairVolumeRefs(ctx context.Context, le *logrus.Entry, path, spaceID string, dryRun bool, lookup refwalk.LookupBlockType) (refRepairResult, error) {
	// Open the stopped volume.
	vol, err := openStoppedVolume(ctx, le, path, dryRun)
	if err != nil {
		return refRepairResult{}, err
	}
	defer vol.Close()

	// Read the replay cursors of the selected Spaces.
	cursors, err := readReplayCursors(ctx, volume_s4db.GetDB(vol), spaceID)
	if err != nil {
		return refRepairResult{}, err
	}
	if spaceID != "" && len(cursors) == 0 {
		return refRepairResult{}, errors.Errorf("no replay cursor found for space %s", spaceID)
	}

	// Start the list rebuild unless this is a dry run, then walk each Space
	// and, for the whole volume, the named roots, and release the walker.
	rr := newRefRepair(le, vol, lookup)
	defer rr.close()
	if !dryRun {
		rr.rebuild, err = vol.OpenListRebuild(ctx)
		rr.res.verified = errors.Is(err, block_gc.ErrListsVerified)
		if err != nil && !rr.res.verified {
			return refRepairResult{}, err
		}
	}
	for _, sc := range cursors {
		if err := rr.walkSpace(ctx, sc); err != nil {
			return refRepairResult{}, errors.Wrapf(err, "walk space %s", sc.spaceID)
		}
	}
	if spaceID == "" {
		if err := rr.walkNamedRoots(ctx); err != nil {
			return refRepairResult{}, errors.Wrap(err, "walk the named roots")
		}
	}
	rr.close()

	// Measure the whole volume before applying any of the planned repairs.
	if err := rr.measurePluginStores(ctx, vol); err != nil {
		return refRepairResult{}, errors.Wrap(err, "count plugin object-store prefix groups")
	}
	if err := rr.measureUntracked(ctx, vol); err != nil {
		return refRepairResult{}, errors.Wrap(err, "measure untracked blocks")
	}

	// Finish the list rebuild unless this is a dry run.
	if rr.rebuild != nil {
		if rr.res.rebuild, err = rr.rebuild.Finish(ctx); err != nil {
			return refRepairResult{}, errors.Wrap(err, "finish the child list rebuild")
		}
	}

	// Split the holders an adoption would refuse.
	if err := rr.splitHolders(ctx, dryRun); err != nil {
		return refRepairResult{}, errors.Wrap(err, "split the holders of listless blocks")
	}
	return rr.res, nil
}

// newDebugRefRepairCommand returns the offline child list repair command.
func newDebugRefRepairCommand() *cli.Command {
	args := &debugRefRepairArgs{}
	return &cli.Command{
		Name:      "ref-repair",
		Usage:     "rebuild the child lists a stopped volume's Spaces and typed roots reach",
		ArgsUsage: "<volume-file>",
		Description: "Walks the Worlds each Space's replay cursor retains, and without --space each named root whose type its writer's rule gives, " +
			"such as an entrypoint World engine's head root in the engine's own bucket, decoding each block by its type; " +
			"it counts the other named roots by kind and decodes none of them. " +
			"It rewrites the child list of each block whose list is missing or lacks a child its decoded refs name, marking it verified. " +
			"When every list a named root reaches is verified it clears the volume's unverified lists mark, so the collector may free older blocks; " +
			"a block it cannot decode keeps its list unverified and is reported, and so does one whose decoded refs drop a child its stored list names. " +
			"A rerun skips the lists it verified, and a volume whose lists are verified is only walked and never repaired. " +
			"Rewriting a list on an adopted volume returns it to unadopted when the mark clears, so the next open recounts every list before anything more frees. " +
			"Blocks stored before puts carried their child lists have none. " +
			"Also counts plugin object-store keys by volume prefix and measures all stored blocks without a child list. " +
			"These counts cover the whole volume even with --space, and describe it before repair. The scan does not decode those blocks. " +
			"Last it counts the holders an adoption would refuse, the named roots and child lists naming a block left stored without a list, " +
			"splitting the lists a named root reaches from leaked ones; a dry run predicts them as a real run would leave the volume. " +
			"Run it only on a stopped volume, with --dry-run first.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
