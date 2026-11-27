//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/sirupsen/logrus"
)

// debugVolumeRefillArgs holds the offline refill's paths and mode.
type debugVolumeRefillArgs struct {
	// from is the stopped s4db file or directory of blocks to read.
	from string
	// refs is the file of block refs to check in order.
	refs string
	// dryRun classifies the fills without writing them.
	dryRun bool
	// lacksOut is the optional output of refs the refill could not restore.
	lacksOut string
	// upgrade decides whether a writing run may upgrade the target format.
	upgrade *FormatUpgrade
}

// BuildFlags returns the refill flags and the shared format upgrade flag.
func (a *debugVolumeRefillArgs) BuildFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.StringFlag{Name: "from", Usage: "stopped source s4db volume file or directory of blocks named by B58 ref", Required: true, Destination: &a.from},
		&cli.StringFlag{Name: "refs", Usage: "file of B58 block refs, one per line, optionally followed by a tab and notes", Required: true, Destination: &a.refs},
		&cli.BoolFlag{Name: "dry-run", Usage: "classify the fills without writing to the target volume", Destination: &a.dryRun},
		&cli.StringFlag{Name: "lacks-out", Usage: "write each missing, corrupt or dropped ref and its reason to this file", Destination: &a.lacksOut},
	}, a.upgrade.BuildFlags()...)
}

// Run refills the stopped target volume and reports the outcome.
func (a *debugVolumeRefillArgs) Run(c *cli.Context) error {
	// Follow the same target open and format policy as other offline writers.
	if c.NArg() != 1 {
		return errors.New("expected one target volume file argument")
	}
	path := c.Args().First()
	res, err := a.refill(c.Context, path)
	if err != nil {
		return err
	}

	// Print the counts only after the volume and the output file have closed.
	writeFields(c.App.Writer, volumeRefillFields(res, a.dryRun))
	return nil
}

// refill streams the requested refs while holding the source and target open.
func (a *debugVolumeRefillArgs) refill(ctx context.Context, path string) (res volumeRefillResult, err error) {
	// Open the inputs before opening the target for a possible format upgrade.
	refs, err := os.Open(a.refs)
	if err != nil {
		return res, errors.Wrap(err, "open refs")
	}
	defer func() { err = stderrors.Join(err, errors.Wrap(refs.Close(), "close refs")) }()

	// Read directory entries directly, or keep one snapshot of a stopped s4db file.
	info, err := os.Stat(a.from)
	if err != nil {
		return res, errors.Wrap(err, "stat refill source")
	}
	var source packcopy.Source = dirBlocks(a.from)
	if !info.IsDir() {
		blocks, closeSource, openErr := openS4DBBlocks(ctx, a.from)
		if openErr != nil {
			return res, openErr
		}
		defer func() { err = stderrors.Join(err, errors.Wrap(closeSource(), "close refill source")) }()
		source = blocks
	}

	// Prepare the output without risking the source, target or input list.
	lacks, closeLacks, err := openVolumeBlockLacks(a.lacksOut, path, a.from, a.refs)
	if err != nil {
		return res, err
	}
	defer func() { err = stderrors.Join(err, closeLacks()) }()

	// The report accounts for the intentional fills without child refs.
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Take the stopped target's usual writer claim, or a read-only open for a preview.
	vol, err := openStoppedVolumeStore(ctx, logrus.NewEntry(logger), path, a.dryRun, a.upgrade.StoreConfig(nil), 0)
	if err != nil {
		return res, err
	}
	defer func() { err = stderrors.Join(err, errors.Wrap(vol.Close(), "close refill target")) }()

	// Stream bounded groups of refs through the target's existing fill path.
	r := volumeRefill{target: vol, source: source, dryRun: a.dryRun, lacks: lacks}
	err = r.run(ctx, refs)
	return r.result, err
}

// newDebugVolumeRefillCommand builds the offline volume refill command.
func newDebugVolumeRefillCommand(upgrade *FormatUpgrade) *cli.Command {
	// Bind the flags and action to the same invocation arguments.
	args := &debugVolumeRefillArgs{upgrade: upgrade}
	return &cli.Command{
		Name:      "volume-refill",
		Usage:     "restore missing block bytes from a stopped s4db copy or block directory",
		ArgsUsage: "<volume-file>",
		Description: "Use a stopped s4db copy of the same volume, or a directory of blocks named by B58 ref, as the source. " +
			"The target decides which verified blocks it can refill. " +
			"The collector releases the volume-refill owner's roots when it sweeps expired fill leases. " +
			"Run with --dry-run to see the counts first.\n\n" +
			"For example, run spacewave debug volume-refill --from backup.s4db --refs missing.tsv --dry-run volume.s4db\n" +
			"For a directory, run spacewave debug volume-refill --from exported-blocks --refs missing.tsv volume.s4db",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
