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
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/volume/packmove"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// newDebugVolumePackIndexMergeCommand builds the command that copies the pack
// index of a store from one s4db file into another as a union. Run it from a
// running backend's volume into a new file to export the index, and from that
// file into another index to restore it there.
func newDebugVolumePackIndexMergeCommand() *cli.Command {
	var worldStore, into string
	return &cli.Command{
		Name:      "volume-pack-index-merge",
		Usage:     "add the packs of a volume file's pack index to another file's index, without duplicates",
		ArgsUsage: "<from.s4db>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "into", Usage: "the s4db file that receives the index, created when missing; no other process may have it open", Destination: &into, Required: true},
			&cli.StringFlag{Name: "world-store", Usage: "the store id the index is keyed by", Value: "hyperline-world", Destination: &worldStore},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return errors.New("expected the volume file to read")
			}
			return mergePackIndex(c.Context, c.Args().Get(0), into, worldStore, os.Stderr)
		},
	}
}

// mergePackIndex merges the pack index of the store in the s4db file from
// into the file into, writing progress lines to w. from is opened read-only
// and may be open in its writer meanwhile.
func mergePackIndex(ctx context.Context, from, into, worldStore string, w io.Writer) error {
	// Open the source beside its writer and the destination for writing.
	src, err := s4db.Open(from, s4db.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := volume_s4db.Open(ctx, logrus.NewEntry(logrus.New()), into, s4db.Options{})
	if err != nil {
		return err
	}
	defer dst.Close()

	// Read the source's index rows through snapshots of the file itself, so
	// the stable reader can refresh the snapshots and see a torn read.
	prefix := block_store_s3.PackIndexPrefix(worldStore)
	read := func(ctx context.Context, fn func(tx kvtx.Tx) error) error {
		return packmove.ReadStable(ctx, src, func(tx kvtx.Tx) error {
			return fn(kvtx_prefixer.NewTx(tx, prefix))
		})
	}

	// Merge, reporting about once a second.
	start := time.Now()
	var last time.Time
	report := func(p block_store_s3.MergeProgress) {
		if time.Since(last) < copyReportInterval {
			return
		}
		last = time.Now()
		_, _ = fmt.Fprintf(w, "%s: %d h rows read, %d written\n", time.Since(start).Round(time.Second), p.Scanned, p.Keys)
	}
	p, err := block_store_s3.MergePackIndex(ctx, kvtx_prefixer.NewPrefixer(dst, prefix), read, report)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "%s: merged %d packs, %d h rows of %d read\n", time.Since(start).Round(time.Second), p.Packs, p.Keys, p.Scanned)
	return nil
}
