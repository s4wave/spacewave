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
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/volume/packmove"
)

// switchArgs are the arguments of the volume-switch command.
type switchArgs struct {
	// worldStore is the bucket ID of the World's block store.
	worldStore string
	// writebackStore is the object store ID of its write-back records.
	writebackStore string
	// logPath is the pack log.
	logPath string
	// shards is the number of key ranges volume-pack ran.
	shards int
	// omitGraph leaves out the reference graph.
	omitGraph bool
	// skipBlocks seeks past the block range without checking it against the log.
	skipBlocks bool
}

// newDebugVolumeSwitchCommand builds the command that moves a stopped volume
// to a new file without the blocks volume-pack wrote.
func newDebugVolumeSwitchCommand() *cli.Command {
	var args switchArgs
	return &cli.Command{
		Name:      "volume-switch",
		Usage:     "copy a stopped volume file to a new file without the blocks volume-pack wrote",
		ArgsUsage: "<volume-file> <new-file>",
		Description: "Copies every key of the stopped volume into a new file at the current store format, converting its " +
			"reference graph on the way and keeping its roots, owner edges and child lists. It leaves out the value of " +
			"each block the pack log committed, found by a pass over the block keys alone. It keeps the value of every " +
			"block outside the World. A World block the log did not commit is copied with a fresh upload marker, so the " +
			"write-back uploads it; a block the log recorded as bad is copied without one. The source's own upload and " +
			"eviction records are left out. The source is opened read-only and never written, and must be closed by " +
			"every other process. A new file that stops early lacks its format version and should be deleted.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "world-store", Usage: "the bucket ID of the World's block store, as volume-pack was given it", Destination: &args.worldStore, Required: true},
			&cli.StringFlag{Name: "writeback-store", Usage: "the object store ID of the World store's write-back records; defaults to <world-store>/writeback", Destination: &args.writebackStore},
			&cli.StringFlag{Name: "log", Usage: "the pack log volume-pack wrote", Destination: &args.logPath, Required: true},
			&cli.IntFlag{Name: "shards", Usage: "the --shards volume-pack ran with; reads the log and its .NN shard logs", Value: 1, Destination: &args.shards},
			&cli.BoolFlag{Name: "omit-graph", Usage: "leave out the reference graph, so the new file's collector starts empty", Destination: &args.omitGraph},
			&cli.BoolFlag{Name: "skip-blocks", Usage: "seek past every block without checking it against the log; only when the logs cover the whole block range", Destination: &args.skipBlocks},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 2 {
				return errors.New("expected the volume file and the new file")
			}
			if args.writebackStore == "" {
				args.writebackStore = args.worldStore + "/writeback"
			}
			return switchVolumeFile(c.Context, c.Args().Get(0), c.Args().Get(1), &args, os.Stderr)
		},
	}
}

// switchVolumeFile copies the stopped volume at src into the new file dst
// without the packed blocks, writing progress lines to w. It removes dst when
// the copy fails.
func switchVolumeFile(ctx context.Context, src, dst string, args *switchArgs, w io.Writer) (err error) {
	// Open the source read-only and refuse a file a daemon has open.
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

	// Create the destination, which must not exist.
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s already exists", dst)
	}
	ddb, err := s4db.Open(dst, s4db.Options{})
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, ddb.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(dst))
		}
	}()

	// Switch, reporting progress at most once per interval.
	keys := store_kvkey.NewDefaultKVKey()
	start := time.Now()
	var last time.Time
	result, err := packmove.Switch(ctx, packmove.SwitchConfig{
		Source:          sdb,
		Dest:            ddb,
		Keys:            keys,
		LogPaths:        packmove.ShardLogPaths(args.logPath, max(args.shards, 1)),
		WorldStoreID:    args.worldStore,
		WritebackPrefix: keys.GetObjectStorePrefixByID(args.writebackStore),
		OmitGraph:       args.omitGraph,
		SkipBlocks:      args.skipBlocks,
		Progress: func(p volume_kvtx.CopyProgress) {
			if time.Since(last) < copyReportInterval {
				return
			}
			last = time.Now()
			fmt.Fprintf(w, "%s: %s\n", time.Since(start).Round(time.Second), formatCopyProgress(p))
		},
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "switched %s to %s in %s: %s\n", src, dst, time.Since(start).Round(time.Second), formatCopyProgress(result.Copy))
	fmt.Fprintf(w, "kept %d blocks outside the World (%s); queued %d unpacked World blocks (%s) for upload; kept %d bad blocks without a marker\n",
		result.Outside, humanize.IBytes(result.OutsideBytes), result.Unpacked, humanize.IBytes(result.UnpackedBytes), result.Bad)
	return nil
}
