//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/dustin/go-humanize"
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume/packmove"
	"github.com/sirupsen/logrus"
)

// Environment variables holding the bucket's credentials. The command never
// prints them.
const (
	packKeyIDEnv = "HYPERLINE_PROD_BUCKET_KEY_ID"
	packKeyEnv   = "HYPERLINE_PROD_BUCKET_KEY" //nolint:gosec // an environment variable name, not a credential.
)

// packArgs are the arguments of the volume-pack command.
type packArgs struct {
	// worldStore is the bucket ID of the World's block store.
	worldStore string
	// logPath is the pack log.
	logPath string
	// bucket, endpoint, region and keyPrefix locate the packs.
	bucket, endpoint, region, keyPrefix string
	// ftps targets an FTPS server instead of the bucket when its host is set.
	ftps ftpsArgs
	// bytesPerSecond limits the upload; zero does not.
	bytesPerSecond int64
	// packMiB is the block data of one pack in MiB; zero uses the default.
	packMiB int
	// skipRest leaves out the count of keys outside the block range.
	skipRest bool
}

// newDebugVolumePackCommand builds the command that packs a volume's World
// blocks beside the running backend.
func newDebugVolumePackCommand() *cli.Command {
	var args packArgs
	return &cli.Command{
		Name:      "volume-pack",
		Usage:     "write a live volume file's World blocks to the bucket as packfiles",
		ArgsUsage: "<volume-file>",
		Description: "Opens the s4db volume file read-only beside the process that writes it: it takes no lock, no reader slot " +
			"and writes no byte of the file. It streams the blocks of the World in key order into packfiles of about " +
			"half the pack limit under --key-prefix in the bucket, with the store's KeepPacks set so nothing is ever " +
			"deleted. A block whose data does not match its hash is in no pack and is counted as bad. Blocks the " +
			"reference graph reaches from any root other than --world-store are left alone. The log records every " +
			"committed block key, so a stopped run resumes where it ended and volume-switch finds the blocks it missed. " +
			"The credentials come from " + packKeyIDEnv + " and " + packKeyEnv + ". With --ftps-host the packs go to that " +
			"FTPS server under --ftps-root instead, with the login from --ftps-netrc. It also counts the keys outside the block " +
			"range and the bytes of their keys and values, which size the file volume-switch writes. " +
			"The upload rate is limited by --bytes-per-second; the command does not watch the backend's load.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "world-store", Usage: "the bucket ID of the World's block store, p/<provider>/<account>/blk/<id>", Destination: &args.worldStore, Required: true},
			&cli.StringFlag{Name: "log", Usage: "the pack log, created when missing and resumed when present", Destination: &args.logPath, Required: true},
			&cli.StringFlag{Name: "bucket", Usage: "the bucket that receives the packs", EnvVars: []string{"HYPERLINE_PROD_BUCKET"}, Destination: &args.bucket},
			&cli.StringFlag{Name: "endpoint", Usage: "the S3 endpoint", Value: "https://s3.us-east-005.backblazeb2.com", EnvVars: []string{"HYPERLINE_PROD_BUCKET_ENDPOINT"}, Destination: &args.endpoint},
			&cli.StringFlag{Name: "region", Usage: "the S3 region", Value: "us-east-005", EnvVars: []string{"HYPERLINE_PROD_BUCKET_REGION"}, Destination: &args.region},
			&cli.StringFlag{Name: "key-prefix", Usage: "the key prefix of the packs in the bucket", Value: "world/packs/", Destination: &args.keyPrefix},
			&cli.Int64Flag{Name: "bytes-per-second", Usage: "limit the block data uploaded per second; zero does not limit", Destination: &args.bytesPerSecond},
			&cli.IntFlag{Name: "pack-mib", Usage: "block data per pack in MiB; zero uses half the pack limit", Destination: &args.packMiB},
			&cli.StringFlag{Name: "ftps-host", Usage: "write the packs to this FTPS server instead of the bucket", Destination: &args.ftps.host},
			&cli.StringFlag{Name: "ftps-netrc", Usage: "the netrc file holding the FTPS login", Destination: &args.ftps.netrc},
			&cli.StringFlag{Name: "ftps-root", Usage: "the FTPS directory holding the key prefix", Destination: &args.ftps.root},
			&cli.IntFlag{Name: "ftps-conns", Usage: "the FTPS connections", Value: block_store_ftps.DefaultConns, Destination: &args.ftps.conns},
			&cli.BoolFlag{Name: "skip-rest", Usage: "do not count the keys outside the block range", Destination: &args.skipRest},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return errors.New("expected the volume file")
			}
			return packVolumeFile(c.Context, c.Args().Get(0), &args, os.Stderr)
		},
	}
}

// packVolumeFile packs the World blocks of the volume file at path, writing
// progress lines to w.
func packVolumeFile(ctx context.Context, path string, args *packArgs, w io.Writer) error {
	// Connect to the FTPS server or the bucket.
	objects, bucket, err := args.objectStore()
	if err != nil {
		return err
	}

	// Build the pack store with KeepPacks, so it never deletes a pack.
	packer := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), objects, bucket, args.keyPrefix, &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer packer.Close()

	// Open the volume file beside its writer.
	vol, err := s4db.Open(path, s4db.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer vol.Close()

	// Pack, reporting progress at most once per interval.
	start := time.Now()
	var last time.Time
	cfg := packmove.PackConfig{
		Source:         vol,
		Keys:           store_kvkey.NewDefaultKVKey(),
		WorldStoreID:   args.worldStore,
		Packer:         packer,
		LogPath:        args.logPath,
		PackBytes:      args.packMiB << 20,
		BytesPerSecond: args.bytesPerSecond,
		SkipRest:       args.skipRest,
		Progress: func(p packmove.PackProgress) {
			if time.Since(last) < copyReportInterval {
				return
			}
			last = time.Now()
			fmt.Fprintf(w, "%s: %s\n", time.Since(start).Round(time.Second), formatPackProgress(p))
		},
	}
	progress, err := packmove.Pack(ctx, cfg)
	fmt.Fprintf(w, "%s: %s\n", time.Since(start).Round(time.Second), formatPackProgress(progress))
	return err
}

// objectStore connects to the FTPS server when --ftps-host is set and to the
// bucket otherwise, returning the store and the bucket to name in it.
func (args *packArgs) objectStore() (block_store_s3.ObjectStore, string, error) {
	// Prefer the FTPS server.
	if args.ftps.host != "" {
		client, err := args.ftps.client()
		return client, args.ftps.root, err
	}

	// Check the bucket and its credentials.
	if args.bucket == "" {
		return nil, "", errors.New("name the bucket with --bucket or HYPERLINE_PROD_BUCKET, or the server with --ftps-host")
	}
	keyID, key := os.Getenv(packKeyIDEnv), os.Getenv(packKeyEnv)
	if keyID == "" || key == "" {
		return nil, "", fmt.Errorf("set %s and %s", packKeyIDEnv, packKeyEnv)
	}

	// Split the scheme from the endpoint, since the client takes a bare host.
	endpoint, insecure := strings.CutPrefix(args.endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	client, err := block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:    endpoint,
		Region:      args.region,
		DisableSsl:  insecure,
		Credentials: &block_store_s3.Credentials{AccessKeyId: keyID, SecretAccessKey: key},
	})
	return client, args.bucket, err
}

// formatPackProgress describes the work of a pack.
func formatPackProgress(p packmove.PackProgress) string {
	// Describe the blocks.
	line := fmt.Sprintf("%d packs, %d blocks, %s packed (%d resumed from the log); %d bad blocks left out; %d blocks outside the World (%s) left in the volume",
		p.Packs, p.Blocks, humanize.IBytes(p.Bytes), p.Resumed, p.Bad, p.Outside, humanize.IBytes(p.OutsideBytes))
	if p.Untracked != 0 {
		line += fmt.Sprintf("; %d packed without references", p.Untracked)
	}
	done := "so far"
	if p.Rest.Done {
		done = "total"
	}
	return fmt.Sprintf("%s; %d keys outside the blocks, %s of keys and values (%s)", line, p.Rest.Keys, humanize.IBytes(p.Rest.Bytes), done)
}
