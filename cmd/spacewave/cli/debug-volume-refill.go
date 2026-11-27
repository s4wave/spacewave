//go:build !js

package spacewave_cli

import (
	"bufio"
	"context"
	stderrors "errors"
	"io"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/sirupsen/logrus"
)

// debugVolumeRefillArgs holds the offline refill's paths and mode.
type debugVolumeRefillArgs struct {
	// from is the stopped s4db file to read.
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
		&cli.StringFlag{Name: "from", Usage: "stopped source s4db volume file", Required: true, Destination: &a.from},
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

	// Open the stopped source at its existing store format.
	source, err := s4db.Open(a.from, s4db.Options{ReadOnly: true})
	if err != nil {
		return res, errors.Wrap(err, "open refill source")
	}
	defer func() { err = stderrors.Join(err, errors.Wrap(source.Close(), "close refill source")) }()
	shared, err := source.Shared()
	if err != nil {
		return res, errors.Wrap(err, "check whether the source volume is stopped")
	}
	if shared {
		return res, errors.Errorf("%s is open in another process. Stop it first", a.from)
	}

	// Keep one read-only snapshot for the entire input stream.
	tx, err := source.NewTransaction(ctx, false)
	if err != nil {
		return res, errors.Wrap(err, "read refill source")
	}
	defer tx.Discard()

	// Prepare the output without risking the source, target or input list.
	lacks := io.Discard
	if a.lacksOut != "" {
		out, err := openVolumeRefillLacks(a.lacksOut, path, a.from, a.refs)
		if err != nil {
			return res, err
		}
		writer := bufio.NewWriter(out)
		defer func() {
			err = stderrors.Join(err, errors.Wrap(writer.Flush(), "flush lacks output"), errors.Wrap(out.Close(), "close lacks output"))
		}()
		lacks = writer
	}

	// The report accounts for the intentional fills without child refs.
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Take the stopped target's usual writer claim, or a read-only open for a preview.
	vol, err := openStoppedVolumeStore(ctx, logrus.NewEntry(logger), path, a.dryRun, a.upgrade.StoreConfig(nil))
	if err != nil {
		return res, err
	}
	defer func() { err = stderrors.Join(err, errors.Wrap(vol.Close(), "close refill target")) }()

	// Stream bounded groups of refs through the target's existing fill path.
	r := volumeRefill{target: vol, source: tx, keys: store_kvkey.NewDefaultKVKey(), dryRun: a.dryRun, lacks: lacks}
	err = r.run(ctx, refs)
	return r.result, err
}

// openVolumeRefillLacks opens a private report and refuses to truncate an input file.
func openVolumeRefillLacks(path string, inputs ...string) (_ *os.File, err error) {
	// Resolve the existing inputs before the output open can create a file.
	files := make([]os.FileInfo, len(inputs))
	for i, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		files[i] = info
	}

	// Inspect the opened output before changing any existing file.
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, errors.Wrap(err, "open lacks output")
	}
	defer func() {
		if err != nil {
			err = stderrors.Join(err, errors.Wrap(out.Close(), "close lacks output"))
		}
	}()
	info, err := out.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "stat lacks output")
	}

	// Check each input by file identity, including links to it.
	for i, input := range files {
		if os.SameFile(info, input) {
			return nil, errors.Errorf("lacks output names an input file %s", inputs[i])
		}
	}

	// Require a private file even when reusing an earlier report.
	if err := out.Chmod(0o600); err != nil {
		return nil, errors.Wrap(err, "set lacks output permissions")
	}
	if err := out.Truncate(0); err != nil {
		return nil, errors.Wrap(err, "clear lacks output")
	}
	return out, nil
}

// newDebugVolumeRefillCommand builds the offline volume refill command.
func newDebugVolumeRefillCommand(upgrade *FormatUpgrade) *cli.Command {
	// Bind the flags and action to the same invocation arguments.
	args := &debugVolumeRefillArgs{upgrade: upgrade}
	return &cli.Command{
		Name:      "volume-refill",
		Usage:     "restore missing block bytes in a stopped volume from another copy",
		ArgsUsage: "<volume-file>",
		Description: "Use a stopped s4db copy of the same volume as the source. The target decides which verified blocks it can refill. " +
			"The collector releases the volume-refill owner's roots when it sweeps expired fill leases. " +
			"Run with --dry-run to see the counts first.\n\n" +
			"For example, run spacewave debug volume-refill --from backup.s4db --refs missing.tsv --dry-run volume.s4db",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
