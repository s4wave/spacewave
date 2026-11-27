//go:build !js

package spacewave_cli

import (
	"bufio"
	"context"
	stderrors "errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/block/packcopy"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world/refwalk"
	"github.com/sirupsen/logrus"
)

// debugRefRepairArgs are the arguments of the offline ref graph repair.
type debugRefRepairArgs struct {
	// spaceID limits the repair to one SharedObject. Empty repairs every Space
	// with a replay cursor in the volume.
	spaceID string
	// dryRun counts the missing edges without writing them.
	dryRun bool
	// peerBlocks is a directory of blocks named by their B58 refs, read in
	// place of the peer for the blocks the volume lacks.
	peerBlocks string
	// syntheticEmpty is a file of B58 refs, one per line, listed with no
	// children when no read decodes them.
	syntheticEmpty string
	// histogram reads every walked block's bytes and reports the blocks and
	// bytes by type and by size.
	histogram bool
	// frontRoots also reports the root that reaches each block the walk
	// left on the front.
	frontRoots bool
	// frontOut names the file for the front listing, empty to omit it.
	frontOut string
	// scanUntyped also reads the blocks below each untyped ref by scanning.
	scanUntyped bool
	// scanVerify verifies the child lists of the blocks read by scanning.
	scanVerify bool
	// upgrade decides whether the open upgrades the volume's format.
	upgrade *FormatUpgrade
}

// BuildFlags returns the ref repair flags.
func (a *debugRefRepairArgs) BuildFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.StringFlag{
			Name:        "space",
			Usage:       "SharedObject ID of the one Space to repair",
			Destination: &a.spaceID,
		},
		&cli.BoolFlag{
			Name:        "dry-run",
			Usage:       "count the blocks lacking edges and the lists a repair would write, without writing them",
			Destination: &a.dryRun,
		},
		&cli.StringFlag{
			Name:        "peer-blocks",
			Usage:       "directory of blocks named by B58 ref, read for the blocks the volume lacks without filling it",
			Destination: &a.peerBlocks,
		},
		&cli.StringFlag{
			Name:        "synthetic-empty",
			Usage:       "file of B58 refs to list with no children when no read decodes them, for a rehearsal on a copy",
			Destination: &a.syntheticEmpty,
		},
		&cli.BoolFlag{
			Name:        "histogram",
			Usage:       "read every walked block's bytes and report blocks and bytes by type and size",
			Destination: &a.histogram,
		},
		&cli.BoolFlag{
			Name: "front-roots",
			Usage: "also walk the stored child lists from every named root and count the blocks the typed walk left on the front by the root that reaches them; " +
				"it holds one key per reached block in memory, so run it on a copy host",
			Destination: &a.frontRoots,
		},
		&cli.StringFlag{
			Name: "front-out",
			Usage: "write the front to a file with tab-separated ref, cause and scan fate, in front order. " +
				"Tabs and line breaks in names and reasons become spaces",
			Destination: &a.frontOut,
		},
		&cli.BoolFlag{
			Name: "scan-untyped",
			Usage: "also read the blocks below each ref the walk cannot type by scanning their bytes for hashes, and compare what the bytes name " +
				"with each stored list. It scans every typed block too and counts the typed refs the scan misses. It verifies nothing unless --scan-verify is given",
			Destination: &a.scanUntyped,
		},
		&cli.BoolFlag{
			Name: "scan-verify",
			Usage: "verify scanned blocks' lists by keeping their stored children and adding hashes known by the volume's bytes or child lists, or by --peer-blocks. " +
				"A block that is not a message needs an authenticating transform and an empty stored list. " +
				"Requires --scan-untyped. Fill unknown blocks before verifying by scan",
			Destination: &a.scanVerify,
		},
	}, a.upgrade.BuildFlags()...)
}

// Run repairs the stopped volume and prints the counts.
func (a *debugRefRepairArgs) Run(c *cli.Context) error {
	// Repair the volume at the argument's path.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	path := c.Args().First()
	le := logrus.NewEntry(logrus.New())
	opts, err := a.opts()
	if err != nil {
		return err
	}
	res, err := repairVolumeRefs(c.Context, le, path, a.spaceID, a.dryRun, space_world_optypes.LookupBlockType, opts, a.upgrade.StoreConfig(nil))
	if err != nil {
		return err
	}

	// Print the repair findings, the volume-wide inventory and the histogram.
	fields := refRepairFields(path, a.dryRun, res)
	if opts.histogram != nil {
		fields = append(fields, histogramFields(opts.histogram)...)
	}
	writeFields(os.Stdout, fields)
	return nil
}

// opts reads the peer and synthetic stand-ins the flags select, with what more
// the run measures.
func (a *debugRefRepairArgs) opts() (refRepairOpts, error) {
	// Require the scan before it can verify lists.
	if a.scanVerify && !a.scanUntyped {
		return refRepairOpts{}, errors.New("--scan-verify requires --scan-untyped")
	}

	// Read the peer store and the synthetic list the flags select.
	opts := refRepairOpts{frontRoots: a.frontRoots, frontOut: a.frontOut, scanUntyped: a.scanUntyped, scanVerify: a.scanVerify}
	if a.peerBlocks != "" {
		opts.peer = packcopy.NewSourceStore([]packcopy.Source{dirBlocks(a.peerBlocks)})
	}
	if a.syntheticEmpty != "" {
		data, err := os.ReadFile(a.syntheticEmpty)
		if err != nil {
			return opts, err
		}
		opts.synthetic = make(map[string]bool)
		for line := range strings.Lines(string(data)) {
			if key := strings.TrimSpace(line); key != "" {
				opts.synthetic[key] = false
			}
		}
	}

	// Make the histogram the run fills.
	if a.histogram {
		opts.histogram = refwalk.NewHistogram()
	}
	return opts, nil
}

// refRepairOpts stands in for what a stopped volume cannot reach offline, and
// selects what more the run measures.
type refRepairOpts struct {
	// frontRoots also walks the stored lists from every named root, as
	// refRepair.measureFront describes.
	frontRoots bool
	// frontOut names the file for the front listing, empty to omit it.
	frontOut string
	// scanUntyped makes the walk scan, as refRepair.scanUntyped describes.
	scanUntyped bool
	// scanVerify verifies scanned lists, as refRepair.visitScanned describes.
	scanVerify bool
	// peer reads the blocks the volume lacks, as the serve reads them from
	// the Space's peers. Nil reads the volume alone.
	peer block.StoreOps
	// synthetic selects blocks to list with no children when no read
	// decodes them, as refRepair.synthetic describes.
	synthetic map[string]bool
	// histogram counts the walked blocks by type and size when set. The walk
	// then reads every block's bytes, leaves included. The root blocks the
	// Space's bucket names, read before the walk, are not counted.
	histogram *refwalk.Histogram
}

// dirBlocks is a directory of blocks named by their B58 refs.
type dirBlocks string

// GetStoredBlock reads the block's file, or returns nil when there is none.
func (d dirBlocks) GetStoredBlock(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, err := os.ReadFile(filepath.Join(string(d), ref.MarshalString()))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &block.StoredBlock{Data: data}, nil
}

// refRepairFields formats the repair findings in the command's field table.
func refRepairFields(path string, dryRun bool, res refRepairResult) [][2]string {
	// A dry run wrote nothing, so its front is the one the volume has now.
	owed := "Reached blocks the walk still owes"
	if dryRun {
		owed = "Front entries before a repair"
	}

	// Keep graph repairs separate from the volume-wide inventory.
	fields := [][2]string{
		{"Volume", path},
		{"Dry run", strconv.FormatBool(dryRun)},
		{"Spaces", strconv.Itoa(res.spaces)},
		{"Blocks walked", strconv.FormatUint(res.blocks, 10)},
		{"Blocks absent", strconv.FormatUint(res.absent, 10)},
		{"Blocks read from the peer", strconv.FormatUint(res.peerRead, 10)},
		{"Blocks listed synthetically empty", strconv.FormatUint(res.synthetic, 10)},
		{"Blocks undecodable", strconv.FormatUint(res.undecodable, 10)},
		{"Blocks without a child list", strconv.FormatUint(res.lacking, 10)},
		{"Children they name", strconv.FormatUint(res.edges, 10)},
		{"Child lists verified", strconv.FormatUint(res.rebuild.Verified, 10)},
		{"Child lists written", strconv.FormatUint(res.rebuild.Rewritten, 10)},
		{"Decoded lists refused for dropping a stored child", strconv.FormatUint(res.rebuild.Dropped, 10)},
		{owed, strconv.FormatUint(res.rebuild.Front, 10)},
		{"Payload roots read as blobs", strconv.FormatUint(res.payloadBlobs, 10)},
		{"Payload roots that are not blobs", strconv.FormatUint(res.notBlob, 10)},
		{"Old generation settled", strconv.FormatBool(res.verified)},
		{"Plugin prefix groups", strconv.Itoa(len(res.pluginStores))},
		{"Stored blocks without a child list", strconv.FormatUint(res.untracked, 10)},
		{"Their bytes", strconv.FormatUint(res.untrackedBytes, 10)},
	}

	// A run that scanned says how the scan compares with the types and lists.
	if res.scan.on {
		fields = append(fields, scanFields(res.scan, res.rooted)...)
	}

	// A run that walked from the named roots says how much they reach.
	if res.rooted {
		fields = append(fields,
			[2]string{"Named roots", strconv.Itoa(res.roots)},
			[2]string{"Listed blocks the named roots reach", strconv.FormatUint(res.rootListed, 10)},
		)
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

	// List the reached blocks the walk still owes, which keep the old
	// generation Open, and the refused reports.
	for _, ref := range res.rebuild.FrontSample {
		fields = append(fields, [2]string{"Owed sample", ref.MarshalString()})
	}
	var frontTotal uint64
	for _, cause := range slices.Sorted(maps.Keys(res.front)) {
		front := res.front[cause]
		frontTotal += front.count
		fields = append(fields, [2]string{"Owed, " + cause, strconv.FormatUint(front.count, 10) + " (" + front.sample.MarshalString() + ")"})
	}
	if res.frontListed {
		fields = append(fields, [2]string{"Front entries listed", strconv.FormatUint(frontTotal, 10)})
	}
	for _, ref := range res.rebuild.DroppedSample {
		fields = append(fields, [2]string{"Refused sample", ref.MarshalString()})
	}

	// List the marked lists found wanting and the payload roots that are
	// not blobs.
	fields = append(fields, markedFields(res.rebuild)...)
	for _, ref := range res.notBlobSample {
		fields = append(fields, [2]string{"Not a blob sample", ref.MarshalString()})
	}

	// Retain the walk's reasons for stopping at an untyped ref, with a sample
	// ref of each.
	for _, reason := range slices.Sorted(maps.Keys(res.untyped)) {
		untyped := res.untyped[reason]
		fields = append(fields, [2]string{"Untyped: " + reason, strconv.FormatUint(untyped.Count, 10) + " (" + untyped.Sample.MarshalString() + ")"})
	}
	return fields
}

// markedFields formats what the rebuild found comparing the marked lists
// with their blocks' content.
func markedFields(res block_gc.ListRebuildResult) [][2]string {
	fields := [][2]string{
		{"Marked lists read", strconv.FormatUint(res.MarkedRead, 10)},
		{"Marked lists matching their content", strconv.FormatUint(res.MarkedMatched, 10)},
		{"Marked lists lacking a decoded child", strconv.FormatUint(res.MarkedLacking, 10)},
		{"Of those, naming a child the decode lacks", strconv.FormatUint(res.MarkedRefused, 10)},
		{"Marked, not decodable", strconv.FormatUint(res.MarkedUndecodable, 10)},
	}
	for _, s := range res.MarkedLackingSample {
		fields = append(fields, [2]string{"Marked lacking sample", s.Ref.MarshalString() + " " + s.Type + " lacks " + s.Missing.MarshalString()})
	}
	for _, s := range res.MarkedUndecodableSample {
		fields = append(fields, [2]string{"Marked, not decodable sample", s.Ref.MarshalString() + " " + s.Type})
	}
	return fields
}

// repairVolumeRefs walks the Spaces of the stopped volume at path, all of them
// or spaceID alone, typing objects' roots with lookup, and unless dryRun is
// set writes the child lists they reach from the decoded blocks, as the typed
// walk of the settle of the volume's old generation, so the collector can
// settle it once nothing more is owed. It compares the marked lists too, as
// block_gc.ListRebuild describes, and a dry run counts what it would write.
// Once the old generation has left Open the rebuild refuses the volume and
// the run only walks it.
func repairVolumeRefs(
	ctx context.Context,
	le *logrus.Entry,
	path, spaceID string,
	dryRun bool,
	lookup refwalk.LookupBlockType,
	opts refRepairOpts,
	storeConf *store_kvtx.Config,
) (refRepairResult, error) {
	// Open the stopped volume.
	vol, err := openStoppedVolumeStore(ctx, le, path, dryRun, storeConf)
	if err != nil {
		return refRepairResult{}, err
	}
	defer vol.Close()

	// Open the optional listing before the walk, so a path that cannot be
	// written fails at once and not after a large Volume's walk. A run that
	// fails removes its partial listing and leaves the path as it was.
	var file *os.File
	var listing *bufio.Writer
	if opts.frontOut != "" {
		if file, err = openFrontOut(path, opts.frontOut); err != nil {
			return refRepairResult{}, err
		}
		defer os.Remove(file.Name())
		defer file.Close()
		listing = bufio.NewWriter(file)
	}

	// Read the replay cursors of the selected Spaces.
	cursors, err := readReplayCursors(ctx, volume_s4db.GetDB(vol), spaceID)
	if err != nil {
		return refRepairResult{}, err
	}
	if len(cursors) == 0 {
		return refRepairResult{}, errors.New("no replay cursor found for the selected Spaces")
	}

	// Make the repair with what the run measures.
	rr := newRefRepair(le, vol, opts.peer, lookup)
	rr.synthetic = opts.synthetic
	rr.scanVerify = opts.scanVerify
	if opts.scanUntyped {
		rr.scanUntyped()
	}
	if opts.histogram != nil {
		rr.histogram = opts.histogram
		rr.walker.KeepBytes()
	}
	defer rr.close()

	// Start the list rebuild, or its dry run, then walk each Space and
	// release the walker.
	if dryRun {
		rr.rebuild = vol.OpenListCheck()
	} else {
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
	if rr.rebuild != nil && !dryRun {
		if err := rr.listUnwalkedSynthetic(ctx); err != nil {
			return refRepairResult{}, err
		}
	}
	rr.close()
	if opts.scanUntyped {
		rr.res.scan.only = rr.walker.ScannedOnly()
	}

	// Measure the whole volume before applying any of the planned repairs.
	if err := rr.measurePluginStores(ctx, vol); err != nil {
		return refRepairResult{}, errors.Wrap(err, "count plugin object-store prefix groups")
	}
	if err := rr.measureUntracked(ctx, vol); err != nil {
		return refRepairResult{}, errors.Wrap(err, "measure untracked blocks")
	}

	// Finish the list rebuild, which a dry run only counts.
	if rr.rebuild != nil {
		if rr.res.rebuild, err = rr.rebuild.Finish(ctx); err != nil {
			return refRepairResult{}, errors.Wrap(err, "finish the child list rebuild")
		}
	}

	// Measure and stream the front in the same walk. A Volume past Open
	// has an empty front.
	var frontErr error
	if rr.rebuild != nil {
		frontErr = rr.measureFront(ctx, dryRun, opts.frontRoots, listing)
	}

	// Move the complete listing into place before returning any report.
	if listing != nil {
		frontErr = stderrors.Join(frontErr, listing.Flush(), file.Sync(), file.Close())
		if frontErr == nil {
			frontErr = os.Rename(file.Name(), opts.frontOut)
		}
	}
	if frontErr != nil {
		return refRepairResult{}, errors.Wrap(frontErr, "measure the front")
	}
	rr.res.frontListed = listing != nil
	return rr.res, nil
}

// openFrontOut creates the file a run writes its front listing to, beside
// outputPath and private to the user. The caller renames it over outputPath
// once the listing is complete, so a file at outputPath is always a whole
// listing. It refuses an outputPath that is the Volume, including an alias
// of the same file, which that rename would replace.
func openFrontOut(volumePath, outputPath string) (*os.File, error) {
	// Compare file identities before creating anything.
	output, err := os.Stat(outputPath)
	switch {
	case err == nil:
		vol, err := os.Stat(volumePath)
		if err != nil {
			return nil, err
		}
		if os.SameFile(vol, output) {
			return nil, errors.Errorf("front listing %q names the volume file", outputPath)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	// Create the private file in the directory the rename stays within.
	return os.CreateTemp(filepath.Dir(outputPath), filepath.Base(outputPath)+".*.tmp")
}

// histogramFields formats a histogram in the command's field table: the total
// counts, each size class, and each type with its blocks in each size class.
func histogramFields(h *refwalk.Histogram) [][2]string {
	// Print the total and the blocks and bytes in each size class.
	total := h.Total()
	fields := [][2]string{
		{"Histogram blocks", strconv.FormatUint(total.Blocks, 10)},
		{"Histogram bytes", strconv.FormatUint(total.Bytes, 10)},
		{"Histogram leaves", strconv.FormatUint(total.Leaves, 10)},
	}
	for class := range refwalk.SizeClasses {
		if total.Sizes[class] == 0 {
			continue
		}
		fields = append(fields, [2]string{
			"Size " + refwalk.SizeClassName(class),
			strconv.FormatUint(total.Sizes[class], 10) + " blocks, " + strconv.FormatUint(total.SizeBytes[class], 10) + " bytes",
		})
	}

	// Print each type, most bytes first, with its blocks in each size class,
	// and mark the blocks only guessed to hold a type.
	for _, row := range h.Rows() {
		name := "Type " + row.Type
		if row.Unsure {
			name += ", unconfirmed"
		}
		var b strings.Builder
		b.WriteString(strconv.FormatUint(row.Blocks, 10) + " blocks, " + strconv.FormatUint(row.Bytes, 10) + " bytes, " + strconv.FormatUint(row.Leaves, 10) + " leaves;")
		for class := range refwalk.SizeClasses {
			if row.Sizes[class] != 0 {
				b.WriteString(" " + refwalk.SizeClassName(class) + " " + strconv.FormatUint(row.Sizes[class], 10))
			}
		}
		fields = append(fields, [2]string{name, b.String()})
	}
	return fields
}

// newDebugRefRepairCommand returns the offline child list repair command.
func newDebugRefRepairCommand(upgrade *FormatUpgrade) *cli.Command {
	args := &debugRefRepairArgs{upgrade: upgrade}
	return &cli.Command{
		Name:      "ref-repair",
		Usage:     "rebuild the child lists a stopped volume's Spaces reach",
		ArgsUsage: "<volume-file>",
		Description: "Walks the Worlds each Space's replay cursor retains, decoding each block by its type, " +
			"and writes each block's child list from its decoded refs, marked as written from content, adding any child the stored list lacks. " +
			"It reports the reached blocks it still owes: once none is owed, the collector settles the volume's old generation and may free older blocks. " +
			"A block it cannot decode stays owed and is reported, and so does one whose decoded refs drop a child its unmarked stored list names. " +
			"It compares lists already marked too, adds a child one lacks whatever else the list names, and reports the marked lists it could not compare. " +
			"An operation payload root that no walked World types is read as a blob only when it certainly is one. " +
			"Otherwise the walk reports it and can read it with --scan-untyped. " +
			"A volume whose old generation has settled is only walked and never repaired. " +
			"A list that gains a child on an adopted volume makes the settle return it to unadopted, so an adoption recounts every list before anything more frees. " +
			"Blocks stored before puts carried their child lists have none. " +
			"With --histogram it reads every walked block's bytes, leaves included, " +
			"and reports the blocks, bytes and leaves by block type and by power-of-two size class. " +
			"Also counts plugin object-store keys by volume prefix and measures all stored blocks without a child list. " +
			"These counts cover the whole volume even with --space, and describe it before repair. The scan does not decode those blocks. " +
			"Run it only on a stopped volume, with --dry-run first.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
