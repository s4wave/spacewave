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
	var reachable bool
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
			"names may be live and would be dropped, unless --allow-unknown is set.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "reachable",
				Usage:       "copy only the blocks and graph records reachable from the graph root",
				Destination: &reachable,
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
			if c.NArg() != 2 {
				return errors.New("expected the volume file and the new file")
			}
			var reach *volume_kvtx.CopyReachableOptions
			if reachable {
				reach = &opts
			}
			return copyVolumeFile(c.Context, c.Args().Get(0), c.Args().Get(1), reach, os.Stderr)
		},
	}
}

// copyVolumeFile copies the stopped volume at src into the new file dst,
// only its reachable part when reachable is set, writing progress lines to w.
// It removes dst when the copy fails.
func copyVolumeFile(ctx context.Context, src, dst string, reachable *volume_kvtx.CopyReachableOptions, w io.Writer) (err error) {
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

	// Copy, reporting progress at most once per interval.
	start := time.Now()
	var last time.Time
	var counts volume_kvtx.CopyProgress
	report := func(p volume_kvtx.CopyProgress) {
		counts = p
		if time.Since(last) < copyReportInterval {
			return
		}
		last = time.Now()
		fmt.Fprintf(w, "%s: %s\n", time.Since(start).Round(time.Second), formatCopyProgress(p))
	}

	// Copy every key, or only the reachable part.
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
