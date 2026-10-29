//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/dustin/go-humanize"
	"github.com/s4wave/spacewave/db/s4db"
)

// newDebugVolumeCompactCommand builds the offline volume compact command.
func newDebugVolumeCompactCommand() *cli.Command {
	return &cli.Command{
		Name:      "volume-compact",
		Usage:     "clean a stopped volume file's chunks and release its free space",
		ArgsUsage: "<volume-file>",
		Description: "Opens a stopped s4db volume and runs one full clean: it moves the live values out of " +
			"every chunk worth cleaning, checkpoints, and releases the freed space to the file system. " +
			"It prints the file's sizes before and after. A daemon cleans in the background on its own; " +
			"this command finishes the work at once, such as after a copy or before measuring.",
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return errors.New("expected one volume file argument")
			}
			return compactVolumeFile(c.Context, c.Args().First(), os.Stderr)
		},
	}
}

// compactVolumeFile runs a full clean of the stopped volume at path, writing
// its sizes before and after to w.
func compactVolumeFile(ctx context.Context, path string, w io.Writer) error {
	// Open the existing file writable, refusing one a daemon has open.
	if _, err := os.Stat(path); err != nil {
		return err
	}
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		return err
	}
	shared, err := db.Shared()
	if err != nil {
		return errors.Join(err, db.Close())
	}
	if shared {
		return errors.Join(fmt.Errorf("%s is open in another process; stop the daemon first", path), db.Close())
	}

	// Measure the file before cleaning.
	before, err := db.Sizes(ctx)
	if err != nil {
		return errors.Join(err, db.Close())
	}
	fmt.Fprintf(w, "before: %s\n", formatSizes(before))

	// Clean, then close so the release reaches the disk, and measure the
	// file again.
	start := time.Now()
	if err := db.Compact(ctx); err != nil {
		return errors.Join(err, db.Close())
	}
	after, err := db.Sizes(ctx)
	if err := errors.Join(err, db.Close()); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "after %s: %s, file length %s\n", time.Since(start).Round(time.Second), formatSizes(after), humanize.IBytes(uint64(info.Size()))) //nolint:gosec // sizes are never negative.
	return nil
}

// formatSizes describes the end of a file's space, which bounds its length,
// and its live values.
func formatSizes(s s4db.Sizes) string {
	return fmt.Sprintf(
		"space %s, live values %s",
		humanize.IBytes(uint64(s.FileBytes)),  //nolint:gosec // sizes are never negative.
		humanize.IBytes(uint64(s.ValueBytes)), //nolint:gosec // sizes are never negative.
	)
}
