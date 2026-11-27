//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/restore"
)

// debugVolumeExportArgs holds the offline export's input and output paths.
type debugVolumeExportArgs struct {
	// refs is the file of B58 refs to export in order.
	refs string
	// out is the private directory receiving verified blocks.
	out string
	// lacksOut is the optional report of missing or corrupt source blocks.
	lacksOut string
}

// BuildFlags returns the source list and private output flags.
func (a *debugVolumeExportArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "refs", Usage: "file of B58 block refs, one per line, optionally followed by a tab and notes", Required: true, Destination: &a.refs},
		&cli.StringFlag{Name: "out", Usage: "private output directory for blocks named by B58 ref", Required: true, Destination: &a.out},
		&cli.StringFlag{Name: "lacks-out", Usage: "write each missing or corrupt ref and its reason to this file", Destination: &a.lacksOut},
	}
}

// Run exports the selected blocks and reports after closing the source and report.
func (a *debugVolumeExportArgs) Run(c *cli.Context) error {
	// Require one stopped source and finish its stream before printing counts.
	if c.NArg() != 1 {
		return errors.New("expected one source volume file argument")
	}
	path := c.Args().First()
	res, err := a.export(c.Context, path)
	if err != nil {
		return err
	}

	// Report the input volume and each outcome in the command's field table.
	writeFields(c.App.Writer, volumeExportFields(path, res))
	return nil
}

// export keeps the source snapshot open while streaming one block at a time.
func (a *debugVolumeExportArgs) export(ctx context.Context, path string) (res restore.ExportResult, err error) {
	// Open the inputs before creating any output.
	refs, err := os.Open(a.refs)
	if err != nil {
		return res, errors.Wrap(err, "open refs")
	}
	defer func() { err = stderrors.Join(err, errors.Wrap(refs.Close(), "close refs")) }()
	source, closeSource, err := restore.OpenS4DBSource(ctx, path)
	if err != nil {
		return res, err
	}
	defer func() { err = stderrors.Join(err, errors.Wrap(closeSource(), "close export source")) }()

	// Require a private directory before writing any block bytes.
	if err := os.MkdirAll(a.out, 0o700); err != nil {
		return res, errors.Wrap(err, "create export directory")
	}
	info, err := os.Stat(a.out)
	if err != nil {
		return res, errors.Wrap(err, "stat export directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return res, errors.Errorf("export directory %s has group or other permission bits; use mode 0700", a.out)
	}

	// Protect the inputs from an optional report's truncation.
	lacks, closeLacks, err := openVolumeBlockLacks(a.lacksOut, path, a.refs)
	if err != nil {
		return res, err
	}
	defer func() { err = stderrors.Join(err, closeLacks()) }()

	// Export without retaining a batch of block bytes or the refs file.
	return restore.Export(ctx, source, refs, a.out, lacks)
}

// newDebugVolumeExportCommand builds the offline block export command.
func newDebugVolumeExportCommand() *cli.Command {
	// Bind flags and execution to one invocation's arguments.
	args := &debugVolumeExportArgs{}
	return &cli.Command{
		Name:      "volume-export",
		Usage:     "export selected verified blocks from a stopped s4db volume",
		ArgsUsage: "<volume-file>",
		Description: "Read a stopped s4db file next to its data and export only the requested blocks. " +
			"The private output directory holds one file per B58 ref and can feed volume-refill --from. " +
			"The source is opened read-only and only the engine's reader-slot bookkeeping writes there.\n\n" +
			"For example, run spacewave debug volume-export --refs missing.tsv --out exported-blocks --lacks-out lacks.tsv backup.s4db",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
