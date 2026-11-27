//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/sirupsen/logrus"
)

// debugVolumeHoldersArgs holds the stopped volume's inspection inputs.
type debugVolumeHoldersArgs struct {
	// refs is the file of B58 refs, optionally followed by tab-separated notes.
	refs string
	// up is the number of parent levels to scan.
	up int
}

// BuildFlags returns the refs input and parent depth flags.
func (a *debugVolumeHoldersArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "refs", Usage: "file of B58 refs, one per line, optionally followed by a tab and notes", Required: true, Destination: &a.refs},
		&cli.IntFlag{Name: "up", Usage: "parent levels to scan (1 through 8)", Value: 1, Destination: &a.up},
	}
}

// Run prints only the requested blocks' structure and their parent relationships.
func (a *debugVolumeHoldersArgs) Run(c *cli.Context) error {
	// Validate the bounded depth before opening the stopped volume.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	if a.up < 1 || a.up > 8 {
		return errors.New("up must be between 1 and 8")
	}
	return a.inspect(c.Context, c.Args().First(), c.App.Writer)
}

// inspect parses the requested refs and keeps the stopped source read-only through the scans.
func (a *debugVolumeHoldersArgs) inspect(ctx context.Context, path string, out io.Writer) (err error) {
	// Parse the same B58 ref column accepted by volume-export.
	input, err := os.Open(a.refs)
	if err != nil {
		return err
	}
	defer func() { err = stderrors.Join(err, input.Close()) }()
	var refs []*block.BlockRef
	if err := streamVolumeBlockRefs(ctx, input, 1, func(batch []*block.BlockRef) error {
		refs = append(refs, batch[0])
		return nil
	}); err != nil {
		return err
	}

	// Open the volume exactly as volume-state does, with conversions refused.
	vol, err := openStoppedVolume(ctx, logrus.NewEntry(logrus.New()), path, volumeReadOnly)
	if err != nil {
		return err
	}
	defer func() { err = stderrors.Join(err, vol.Close()) }()
	if _, err := io.WriteString(out, "level\tref\tchild\tholders\tripe\tlisted\tmarked\tstored\tqueued\treleased\tparked\tfront\tfront cause\n"); err != nil {
		return err
	}

	// Print each relationship immediately while the owning package scans.
	levels, err := vol.InspectHolders(ctx, refs, a.up, func(level int, child *block.BlockRef, row block_gc.HolderRow) error {
		// Include the child relationship and only label a cause for actual front entries.
		childRef, cause := "", ""
		if child != nil {
			childRef = child.MarshalString()
		}
		if row.Front {
			cause = row.Cause.String()
		}
		fields := []string{
			strconv.Itoa(level), row.Ref.MarshalString(), childRef,
			strconv.FormatUint(row.Holders, 10), strconv.FormatUint(row.Ripe, 10),
			strconv.FormatBool(row.Listed), strconv.FormatBool(row.Marked), strconv.FormatBool(row.Stored),
			strconv.FormatBool(row.Queued), strconv.FormatBool(row.Released), strconv.FormatBool(row.Parked), strconv.FormatBool(row.Front), cause,
		}
		_, err := io.WriteString(out, strings.Join(fields, "\t")+"\n")
		return err
	})
	if err != nil {
		return err
	}

	// Count emitted rows by level and proof state, including parents beyond the retained eight.
	for level, counts := range levels {
		parts := []string{strconv.FormatUint(counts.Rows, 10) + " rows"}
		for cause, count := range counts.States {
			if count != 0 {
				parts = append(parts, strconv.FormatUint(count, 10)+" "+block_gc.FrontCause(cause).String())
			}
		}
		parts = append(parts, strconv.Itoa(counts.Followed)+" distinct parents retained (first 8 per ref)")
		if _, err := io.WriteString(out, "# level "+strconv.Itoa(level)+": "+strings.Join(parts, "; ")+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// newDebugVolumeHoldersCommand returns the read-only holder-chain diagnostic.
func newDebugVolumeHoldersCommand() *cli.Command {
	args := &debugVolumeHoldersArgs{}
	return &cli.Command{
		Name:      "volume-holders",
		Usage:     "print block counts and parent lists in a stopped volume",
		ArgsUsage: "<volume-file>",
		Description: "Opens the stopped volume read-only and prints structural records, never block bytes. " +
			"Scans child lists once per level in key order, printing every matching parent and its child. " +
			"Follows only the first eight parents per ref at the next level, up to --up levels. " +
			"Released lists are included and identified by the released column. It writes nothing to the volume.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
