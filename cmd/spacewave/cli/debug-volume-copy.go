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
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// copyReportInterval is the least time between progress lines.
const copyReportInterval = 10 * time.Second

// newDebugVolumeCopyCommand builds the offline volume copy command.
func newDebugVolumeCopyCommand() *cli.Command {
	// Bind the flags.
	var reachable, verify bool
	var opts volume_kvtx.CopyReachableOptions
	return &cli.Command{
		Name:      "volume-copy",
		Usage:     "copy a stopped volume file into a new file at the current store and file formats",
		ArgsUsage: "<volume-file> <new-file>",
		Description: "Copies every key of a stopped s4db volume into a new file, converting a Cayley reference graph " +
			"to edge records on the way, so the source is never rewritten in place. It reads a source in the previous s4db " +
			"value format and writes the new file in the current one. The new file is written in key order " +
			"with compaction off and records the format version last; a copy that stops early lacks it and should be deleted. " +
			"The source is left unchanged. Replace the source with the new file only after checking it. " +
			"With --reachable, the source must be at store format 3 or later, and the copy keeps only the blocks " +
			"and graph records reachable from the graph root, as a complete collection would. Check its count of " +
			"unowned blocks before using it, counted with --count-unowned: the running collector never deletes those. " +
			"A reachable copy refuses a source whose walk reaches a block without a child list, since the blocks it " +
			"names may be live and would be dropped, unless --allow-unknown is set. " +
			"With --verify, the new file is reopened after the copy and compared with the source key by key; " +
			"a new file that fails the check is kept for inspection.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "reachable",
				Usage:       "copy only the blocks and graph records reachable from the graph root",
				Destination: &reachable,
			},
			&cli.BoolFlag{
				Name:        "verify",
				Usage:       "reopen the new file and compare every key and value with the source",
				Destination: &verify,
			},
			&cli.BoolFlag{
				Name:        "allow-unknown",
				Usage:       "with --reachable, copy even when the walk reaches blocks without a child list, dropping the blocks they name",
				Destination: &opts.AllowUnknown,
			},
			&cli.BoolFlag{
				Name:        "count-unowned",
				Usage:       "with --reachable, count the dropped blocks no graph record owns, at one index lookup each",
				Destination: &opts.CountUnowned,
			},
		},
		Action: func(c *cli.Context) error {
			// Check the arguments, then copy and verify.
			if c.NArg() != 2 {
				return errors.New("expected the volume file and the new file")
			}
			if reachable && verify {
				return errors.New("--verify compares a whole copy, so it cannot check a --reachable one")
			}
			var reach *volume_kvtx.CopyReachableOptions
			if reachable {
				reach = &opts
			}
			src, dst := c.Args().Get(0), c.Args().Get(1)
			if err := copyVolumeFile(c.Context, src, dst, reach, verify, os.Stderr); err != nil || !verify {
				return err
			}
			return verifyVolumeFile(c.Context, src, dst, os.Stderr)
		},
	}
}

// copyVolumeFile copies the stopped volume at src into the new file dst,
// only its reachable part when reachable is set, writing progress lines to w.
// With verify, it first checks that a copy of src can be verified. It removes
// dst when the copy fails.
func copyVolumeFile(ctx context.Context, src, dst string, reachable *volume_kvtx.CopyReachableOptions, verify bool, w io.Writer) (err error) {
	// Open the source read-only, which also reads the previous value format,
	// refusing a file a daemon has open.
	sdb, err := s4db.Open(src, s4db.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer sdb.Close()
	shared, err := sdb.Shared()
	if err != nil {
		return err
	}
	if shared {
		return fmt.Errorf("%s is open in another process; stop the daemon first", src)
	}
	if verify {
		if err := volume_kvtx.CheckVerifiable(ctx, sdb, store_kvkey.NewDefaultKVKey()); err != nil {
			return fmt.Errorf("%w; copy it without --verify", err)
		}
	}

	// Create the destination, which must not exist. Its keys arrive in order
	// and never move, so compaction has nothing to do.
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s already exists", dst)
	}
	ddb, err := s4db.Open(dst, s4db.Options{RelocateBudget: -1})
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, ddb.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(dst))
		}
	}()

	// Copy every key, or only the reachable part.
	start := time.Now()
	var counts volume_kvtx.CopyProgress
	report := newCopyReporter(w, start, &counts)
	copyStore := volume_kvtx.CopyStore
	if reachable != nil {
		copyStore = func(ctx context.Context, src, dst kvtx.Store, keys *store_kvkey.KVKey, progress func(volume_kvtx.CopyProgress)) error {
			return volume_kvtx.CopyReachable(ctx, src, dst, keys, *reachable, progress)
		}
	}
	if err := copyStore(ctx, sdb, ddb, store_kvkey.NewDefaultKVKey(), report); err != nil {
		return err
	}
	fmt.Fprintf(w, "copied %s to %s in %s: %s\n", src, dst, time.Since(start).Round(time.Second), formatCopyProgress(counts))
	return nil
}

// verifyVolumeFile reopens the stopped volume at src and its copy at dst
// read-only and compares them key by key, writing progress lines to w. It
// leaves dst in place when they differ.
func verifyVolumeFile(ctx context.Context, src, dst string, w io.Writer) error {
	// Open both files read-only.
	sdb, err := s4db.Open(src, s4db.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer sdb.Close()
	ddb, err := s4db.Open(dst, s4db.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer ddb.Close()

	// Compare, reporting progress at most once per interval.
	start := time.Now()
	var counts volume_kvtx.CopyProgress
	if err := volume_kvtx.VerifyCopy(ctx, sdb, ddb, store_kvkey.NewDefaultKVKey(), newCopyReporter(w, start, &counts)); err != nil {
		return fmt.Errorf("verify %s against %s, which is kept: %w", dst, src, err)
	}
	fmt.Fprintf(w, "verified %s against %s in %s: %s\n", dst, src, time.Since(start).Round(time.Second), formatCopyProgress(counts))
	return nil
}

// newCopyReporter returns a progress callback that stores each report in
// counts and writes a line to w at most once per copyReportInterval.
func newCopyReporter(w io.Writer, start time.Time, counts *volume_kvtx.CopyProgress) func(volume_kvtx.CopyProgress) {
	var last time.Time
	return func(p volume_kvtx.CopyProgress) {
		*counts = p
		if time.Since(last) < copyReportInterval {
			return
		}
		last = time.Now()
		fmt.Fprintf(w, "%s: %s\n", time.Since(start).Round(time.Second), formatCopyProgress(p))
	}
}

// formatCopyProgress describes the work of a copy, with what a reachable copy
// dropped.
func formatCopyProgress(p volume_kvtx.CopyProgress) string {
	line := fmt.Sprintf("%d keys, %s, %d graph edges", p.Keys, humanize.IBytes(p.Bytes), p.Edges)
	if p.UnknownBlocks != 0 {
		line = fmt.Sprintf("%s; %d reached blocks without a child list", line, p.UnknownBlocks)
	}
	if p.DroppedBytes == 0 {
		return line
	}
	return fmt.Sprintf(
		"%s; dropped %d blocks (%d unowned), %d graph records, %s of keys",
		line, p.DroppedBlocks, p.UnownedBlocks, p.DroppedRecords, humanize.IBytes(p.DroppedBytes),
	)
}
