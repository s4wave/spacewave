//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/dustin/go-humanize"
	"github.com/s4wave/spacewave/db/block"
	block_store_sql "github.com/s4wave/spacewave/db/block/store/sql"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// sqlPasswordEnv names the variable holding the SQL block store's password.
const sqlPasswordEnv = "SPACEWAVE_SQL_PASSWORD"

// uploadMarkerInfix precedes the hash in the key of a writeback store's
// upload marker, under the volume's object stores.
var uploadMarkerInfix = []byte("/writeback/marked/")

// copyReportInterval is the least time between progress lines.
const copyReportInterval = 10 * time.Second

// newDebugVolumeCopyCommand builds the offline volume copy command.
func newDebugVolumeCopyCommand() *cli.Command {
	// Bind the flags.
	var reachable bool
	var opts volume_kvtx.CopyReachableOptions
	var dropSQL bool
	var sqlArgs sqlDrop
	var sqlPort uint
	return &cli.Command{
		Name:      "volume-copy",
		Usage:     "copy a stopped volume file into a new file at the current store format",
		ArgsUsage: "<volume-file> <new-file>",
		Description: "Copies every key of a stopped s4db volume into a new file, converting a Cayley reference graph " +
			"to edge records on the way, so the source is never rewritten in place. The new file is written in key order " +
			"with compaction off and records the format version last; a copy that stops early lacks it and should be deleted. " +
			"The source is left unchanged. Replace the source with the new file only after checking it. " +
			"With --reachable, the source must be at store format 3 or later, and the copy keeps only the blocks " +
			"and graph records reachable from the graph root, as a complete collection would. Check its count of " +
			"unowned blocks before using it, counted with --count-unowned: the running collector never deletes those. " +
			"A reachable copy refuses a source whose walk reaches a block without a child list, since the blocks it " +
			"names may be live and would be dropped, unless --allow-unknown is set. " +
			"With --drop-sql-blocks, the copy keeps every key outside the blocks and leaves out the blocks the SQL " +
			"block store already holds, except blocks with an upload marker. The password is read from " + sqlPasswordEnv + ".",
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
			&cli.BoolFlag{
				Name:        "drop-sql-blocks",
				Usage:       "leave out the blocks without an upload marker that the SQL block store holds",
				Destination: &dropSQL,
			},
			&cli.StringFlag{Name: "sql-host", Usage: "with --drop-sql-blocks, the database host", Destination: &sqlArgs.conf.Host},
			&cli.UintFlag{Name: "sql-port", Usage: "with --drop-sql-blocks, the database port; zero uses 3306", Destination: &sqlPort},
			&cli.StringFlag{Name: "sql-database", Usage: "with --drop-sql-blocks, the database name", Destination: &sqlArgs.conf.Database},
			&cli.StringFlag{Name: "sql-user", Usage: "with --drop-sql-blocks, the database user", Destination: &sqlArgs.conf.User},
			&cli.StringFlag{Name: "sql-table", Usage: "with --drop-sql-blocks, the blocks table", Value: "blocks", Destination: &sqlArgs.conf.Table},
			&cli.BoolFlag{Name: "sql-tls", Usage: "with --drop-sql-blocks, connect with TLS", Value: true, Destination: &sqlArgs.conf.TLS},
			&cli.StringFlag{Name: "sql-tls-cert-sha256", Usage: "with --drop-sql-blocks, the hex SHA-256 of the server certificate to trust", Destination: &sqlArgs.conf.TLSCertSHA256},
			&cli.StringFlag{Name: "sql-store-id", Usage: "with --drop-sql-blocks, the block store id whose rows to check", Destination: &sqlArgs.storeID},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 2 {
				return errors.New("expected the volume file and the new file")
			}
			var reach *volume_kvtx.CopyReachableOptions
			if reachable {
				reach = &opts
			}
			var drop *sqlDrop
			if dropSQL {
				if reachable {
					return errors.New("--drop-sql-blocks copies every key outside the blocks; drop --reachable")
				}
				sqlArgs.conf.Port = uint32(sqlPort) //nolint:gosec // a port fits.
				sqlArgs.conf.Password = os.Getenv(sqlPasswordEnv)
				sqlArgs.conf.MaxConns = 1
				if sqlArgs.conf.Host == "" || sqlArgs.conf.Database == "" || sqlArgs.conf.User == "" || sqlArgs.storeID == "" {
					return errors.New("--drop-sql-blocks needs --sql-host, --sql-database, --sql-user and --sql-store-id")
				}
				drop = &sqlArgs
			}
			return copyVolumeFile(c.Context, c.Args().Get(0), c.Args().Get(1), reach, drop, os.Stderr)
		},
	}
}

// copyVolumeFile copies the stopped volume at src into the new file dst,
// only its reachable part when reachable is set, or without the blocks SQL
// holds when drop is set, writing progress lines to w. It removes dst when
// the copy fails.
func copyVolumeFile(ctx context.Context, src, dst string, reachable *volume_kvtx.CopyReachableOptions, drop *sqlDrop, w io.Writer) (err error) {
	// Open the source, refusing a missing file, which the open would create,
	// and a file a daemon has open.
	if _, err := os.Stat(src); err != nil {
		return err
	}
	sdb, err := s4db.Open(src, s4db.Options{})
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

	// Copy every key, only the reachable part, or every key but the blocks
	// SQL holds.
	copyStore := volume_kvtx.CopyStore
	if drop != nil {
		check, closeSQL, err := drop.open(ctx, sdb, store_kvkey.NewDefaultKVKey(), w)
		if err != nil {
			return err
		}
		defer closeSQL()
		copyStore = func(ctx context.Context, src, dst kvtx.Store, keys *store_kvkey.KVKey, progress func(volume_kvtx.CopyProgress)) error {
			return volume_kvtx.CopyStoreDropping(ctx, src, dst, keys, check, progress)
		}
	}
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

// sqlDrop locates the SQL block store a dropping copy checks.
type sqlDrop struct {
	// conf locates the database and its blocks table.
	conf block_store_sql.Config
	// storeID is the block store whose rows are checked.
	storeID string
}

// open reads the upload markers of the volume src and returns the copy's drop
// check with the function that closes its database.
//
// The check keeps every marked block and drops an unmarked block only when
// the SQL store holds it. A marker does not mean SQL lacks the block: a crash
// between the upload and the marker's clear leaves a marked block SQL holds.
// No marker does not mean SQL holds it either: blocks written before the
// store was placed on SQL, or while uploads were off, were never marked.
func (d *sqlDrop) open(ctx context.Context, src kvtx.Store, keys *store_kvkey.KVKey, w io.Writer) (volume_kvtx.DropBlocks, func(), error) {
	// Read the hashes of every writeback store's marked blocks.
	marked := make(map[string]struct{})
	tx, err := src.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	err = tx.ScanPrefixKeys(ctx, keys.GetObjectStoreFullPrefix(), func(key []byte) error {
		if _, name, ok := bytes.Cut(key, uploadMarkerInfix); ok {
			marked[string(name)] = struct{}{}
		}
		return nil
	})
	tx.Discard()
	if err != nil {
		return nil, nil, fmt.Errorf("read the upload markers: %w", err)
	}
	fmt.Fprintf(w, "keeping the blocks of %d upload markers\n", len(marked))

	// Open the store's rows.
	db, err := block_store_sql.Open(&d.conf)
	if err != nil {
		return nil, nil, err
	}
	store := block_store_sql.NewStore(logrus.NewEntry(logrus.New()), db, d.conf.Table, d.storeID)

	// Drop the unmarked blocks the store holds, checked in one batch.
	check := func(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
		drop := make([]bool, len(refs))
		var ask []*block.BlockRef
		var at []int
		for i, ref := range refs {
			if _, ok := marked[ref.GetHash().MarshalString()]; !ok {
				ask = append(ask, ref)
				at = append(at, i)
			}
		}
		if len(ask) == 0 {
			return drop, nil
		}
		held, err := store.GetBlockExistsBatch(ctx, ask)
		if err != nil {
			return nil, err
		}
		for j, i := range at {
			drop[i] = held[j]
		}
		return drop, nil
	}
	return check, store.Close, nil
}
