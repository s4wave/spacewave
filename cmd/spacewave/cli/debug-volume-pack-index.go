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
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// packIndexArgs are the arguments of the volume-pack-index command.
type packIndexArgs struct {
	pack packArgs
	// width is the packs read at once.
	width int
}

// newDebugVolumePackIndexCommand builds the command that fills the pack index
// of a volume file from the packs a store holds, so the backend that opens the
// file reads no entry and no pack index at its start.
func newDebugVolumePackIndexCommand() *cli.Command {
	var args packIndexArgs
	return &cli.Command{
		Name:      "volume-pack-index",
		Usage:     "build the entry cache and block location index of the packs into a stopped volume file",
		ArgsUsage: "<volume.s4db>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "world-store", Usage: "the World block store's id in its account, as the backend logs it in bstore-id, such as hyperline-world; the index is keyed by it", Destination: &args.pack.worldStore, Required: true},
			&cli.StringFlag{Name: "bucket", Usage: "the bucket that holds the packs", EnvVars: []string{"HYPERLINE_PROD_BUCKET"}, Destination: &args.pack.bucket},
			&cli.StringFlag{Name: "endpoint", Usage: "the S3 endpoint", Value: "https://s3.us-east-005.backblazeb2.com", EnvVars: []string{"HYPERLINE_PROD_BUCKET_ENDPOINT"}, Destination: &args.pack.endpoint},
			&cli.StringFlag{Name: "region", Usage: "the S3 region", Value: "us-east-005", EnvVars: []string{"HYPERLINE_PROD_BUCKET_REGION"}, Destination: &args.pack.region},
			&cli.StringFlag{Name: "key-prefix", Usage: "the key prefix of the packs in the bucket", Value: "world/packs/", Destination: &args.pack.keyPrefix},
			&cli.StringFlag{Name: "ftps-host", Usage: "read the packs from this FTPS server instead of the bucket", Destination: &args.pack.ftps.host},
			&cli.StringFlag{Name: "ftps-netrc", Usage: "the netrc file holding the FTPS logins", Destination: &args.pack.ftps.netrc},
			&cli.StringFlag{Name: "ftps-root", Usage: "the FTPS directory holding the key prefix", Destination: &args.pack.ftps.root},
			&cli.IntFlag{Name: "ftps-conns", Usage: "the most FTPS sessions open at once across the netrc logins, spares included", Value: block_store_ftps.DefaultMaxSessions, Destination: &args.pack.ftps.conns},
			&cli.IntFlag{Name: "width", Usage: "packs read at once; zero uses the FTPS sessions or 8", Destination: &args.width},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return errors.New("expected the volume file")
			}
			return buildPackIndex(c.Context, c.Args().Get(0), &args, os.Stderr)
		},
	}
}

// buildPackIndex lists the packs of the store, reads the entries and the index
// tail of each pack, and writes them into the volume file at path, which the
// backend must not have open. It resumes from what the file holds.
func buildPackIndex(ctx context.Context, path string, args *packIndexArgs, w io.Writer) error {
	objects, bucket, err := args.pack.objectStore()
	if err != nil {
		return err
	}
	vol, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		return err
	}
	defer vol.Close()

	// Build the store the backend builds, over the same key space.
	index := kvtx_prefixer.NewPrefixer(vol, block_store_s3.PackIndexPrefix(args.pack.worldStore))
	packs := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), objects, bucket, args.pack.keyPrefix, &block_store_s3.PackStoreOptions{
		KeepPacks: true,
		Index:     index,
	})
	defer packs.Close()

	width := args.width
	if width <= 0 {
		width = 8
		if args.pack.ftps.host != "" {
			width = max(args.pack.ftps.conns-1, 1)
		}
	}
	start := time.Now()
	report := func(p block_store_s3.IndexProgress) {
		secs := max(time.Since(start).Seconds(), 0.001)
		_, _ = fmt.Fprintf(w, "%d packs, %d keys, %s read; %.1f packs/s, %.0f keys/s, %s/s\n",
			p.Packs, p.Keys, humanize.IBytes(p.Bytes),
			float64(p.Packs)/secs, float64(p.Keys)/secs, humanize.IBytes(uint64(float64(p.Bytes)/secs)))
	}
	return packs.BuildIndex(ctx, width, report)
}
