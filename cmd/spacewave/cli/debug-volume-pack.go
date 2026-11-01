//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"

	block_store_sql "github.com/s4wave/spacewave/db/block/store/sql"
	"golang.org/x/sync/errgroup"

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
	// uploads is the number of packs uploading at once in each shard.
	uploads int
	// shards is the number of key ranges each pass packs in parallel.
	shards int
	// skipRest leaves out the count of keys outside the block range.
	skipRest bool
	// sqlLog, when set, also packs the SQL block store's rows the volume
	// lacks, logging them there.
	sqlLog string
	// sql locates the SQL block store, sqlPort its port and sqlStoreID its
	// block store id.
	sql        block_store_sql.Config
	sqlPort    uint
	sqlStoreID string
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
			"The upload rate is limited by --bytes-per-second; the command does not watch the backend's load. " +
			"With --sql-log it also packs, beside the volume's blocks, the rows of the SQL block store the volume lacks, " +
			"reading the password from " + sqlPasswordEnv + "; run it with the backend stopped.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "world-store", Usage: "the bucket ID of the World's block store, p/<provider>/<account>/blk/<id>", Destination: &args.worldStore, Required: true},
			&cli.StringFlag{Name: "log", Usage: "the pack log, created when missing and resumed when present", Destination: &args.logPath, Required: true},
			&cli.StringFlag{Name: "bucket", Usage: "the bucket that receives the packs", EnvVars: []string{"HYPERLINE_PROD_BUCKET"}, Destination: &args.bucket},
			&cli.StringFlag{Name: "endpoint", Usage: "the S3 endpoint", Value: "https://s3.us-east-005.backblazeb2.com", EnvVars: []string{"HYPERLINE_PROD_BUCKET_ENDPOINT"}, Destination: &args.endpoint},
			&cli.StringFlag{Name: "region", Usage: "the S3 region", Value: "us-east-005", EnvVars: []string{"HYPERLINE_PROD_BUCKET_REGION"}, Destination: &args.region},
			&cli.StringFlag{Name: "key-prefix", Usage: "the key prefix of the packs in the bucket", Value: "world/packs/", Destination: &args.keyPrefix},
			&cli.Int64Flag{Name: "bytes-per-second", Usage: "limit the block data uploaded per second; zero does not limit", Destination: &args.bytesPerSecond},
			&cli.IntFlag{Name: "pack-mib", Usage: "block data per pack in MiB; zero uses half the pack limit", Destination: &args.packMiB},
			&cli.IntFlag{Name: "uploads", Usage: "packs uploading at once in each shard", Value: 8, Destination: &args.uploads},
			&cli.IntFlag{Name: "shards", Usage: "key ranges each pass packs in parallel, split on the block hash; shard i > 0 logs to the log path with .NN appended", Value: 1, Destination: &args.shards},
			&cli.StringFlag{Name: "ftps-host", Usage: "write the packs to this FTPS server instead of the bucket", Destination: &args.ftps.host},
			&cli.StringFlag{Name: "ftps-netrc", Usage: "the netrc file holding the FTPS login", Destination: &args.ftps.netrc},
			&cli.StringFlag{Name: "ftps-root", Usage: "the FTPS directory holding the key prefix", Destination: &args.ftps.root},
			&cli.IntFlag{Name: "ftps-conns", Usage: "the most FTPS sessions open at once across the netrc logins, spares included", Value: block_store_ftps.DefaultMaxSessions, Destination: &args.ftps.conns},
			&cli.BoolFlag{Name: "skip-rest", Usage: "do not count the keys outside the block range", Destination: &args.skipRest},
			&cli.StringFlag{Name: "sql-log", Usage: "also pack the SQL block store's rows the volume lacks, with this log", Destination: &args.sqlLog},
			&cli.StringFlag{Name: "sql-host", Usage: "with --sql-log, the database host", Destination: &args.sql.Host},
			&cli.UintFlag{Name: "sql-port", Usage: "with --sql-log, the database port; zero uses 3306", Destination: &args.sqlPort},
			&cli.StringFlag{Name: "sql-database", Usage: "with --sql-log, the database name", Destination: &args.sql.Database},
			&cli.StringFlag{Name: "sql-user", Usage: "with --sql-log, the database user", Destination: &args.sql.User},
			&cli.StringFlag{Name: "sql-table", Usage: "with --sql-log, the blocks table", Value: "blocks", Destination: &args.sql.Table},
			&cli.BoolFlag{Name: "sql-tls", Usage: "with --sql-log, connect with TLS", Value: true, Destination: &args.sql.TLS},
			&cli.StringFlag{Name: "sql-tls-cert-sha256", Usage: "with --sql-log, the hex SHA-256 of the server certificate to trust", Destination: &args.sql.TLSCertSHA256},
			&cli.StringFlag{Name: "sql-store-id", Usage: "with --sql-log, the block store id of the World's rows", Destination: &args.sqlStoreID},
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
	cfg := packmove.PackConfig{
		Source:         vol,
		Keys:           store_kvkey.NewDefaultKVKey(),
		WorldStoreID:   args.worldStore,
		Packer:         packer,
		LogPath:        args.logPath,
		PackBytes:      args.packMiB << 20,
		BytesPerSecond: args.bytesPerSecond,
		SkipRest:       args.skipRest,
		Uploads:        args.uploads,
	}
	g, gctx := errgroup.WithContext(ctx)
	firstBlock, err := firstVolumeBlock(ctx, vol, cfg.Keys.GetBlockFullPrefix())
	if err != nil {
		return err
	}
	runShards(g, gctx, w, start, "volume", cfg, firstBlock, max(args.shards, 1), packmove.Pack)

	// Open the SQL block store's rows.
	if args.sqlLog != "" {
		args.sql.Port = uint32(args.sqlPort) //nolint:gosec // a port fits.
		args.sql.Password = os.Getenv(sqlPasswordEnv)
		args.sql.MaxConns = 4 * max(args.shards, 1)
		if args.sql.Host == "" || args.sql.Database == "" || args.sql.User == "" || args.sqlStoreID == "" {
			return errors.New("--sql-log needs --sql-host, --sql-database, --sql-user and --sql-store-id")
		}
		db, err := block_store_sql.Open(&args.sql)
		if err != nil {
			return err
		}
		rows := block_store_sql.NewStore(logrus.NewEntry(logrus.New()), db, args.sql.Table, args.sqlStoreID)
		defer rows.Close()

		// Pack the rows beside the volume, with their own logs.
		firstRow, err := rows.ListHashes(ctx, nil, nil, 1)
		if err != nil {
			return err
		}
		sqlCfg := cfg
		sqlCfg.SQL = rows
		sqlCfg.LogPath = args.sqlLog
		if len(firstRow) != 0 {
			runShards(g, gctx, w, start, "sql", sqlCfg, firstRow[0], max(args.shards, 1), packmove.PackSQL)
		}
	}
	return g.Wait()
}

// digestLen is the length of the block hash digest that ends every block and
// row key; the shards split on its first byte.
const digestLen = 32

// runShards starts n runs of pack over the key ranges that split the digest
// of keys shaped like first, and reports their summed progress under name.
// Shard 0 logs to cfg.LogPath and shard i to cfg.LogPath with .NN appended.
func runShards(
	g *errgroup.Group,
	ctx context.Context,
	w io.Writer,
	start time.Time,
	name string,
	cfg packmove.PackConfig,
	first []byte,
	n int,
	pack func(context.Context, packmove.PackConfig) (packmove.PackProgress, error),
) {
	// Bound each shard by the first digest byte.
	var bounds [][]byte
	keyPrefix := first[:max(len(first)-digestLen, 0)]
	for i := 1; i < n; i++ {
		bounds = append(bounds, append(bytes.Clone(keyPrefix), byte(i*256/n))) //nolint:gosec // below 256.
	}
	bounds = append([][]byte{nil}, append(bounds, nil)...)

	// Sum the shards' progress and report it at most once per interval.
	var mtx sync.Mutex
	var last time.Time
	shards := make([]packmove.PackProgress, n)
	report := func(i int, p packmove.PackProgress, done string) {
		// Record the shard's progress, holding the report for the interval.
		mtx.Lock()
		defer mtx.Unlock()
		shards[i] = p
		if done == "" && time.Since(last) < copyReportInterval {
			return
		}
		last = time.Now()

		// Print the sum over every shard.
		var sum packmove.PackProgress
		for _, s := range shards {
			sum.Packs += s.Packs
			sum.Blocks += s.Blocks
			sum.Bytes += s.Bytes
			sum.Bad += s.Bad
			sum.Untracked += s.Untracked
			sum.Resumed += s.Resumed
			sum.Outside = max(sum.Outside, s.Outside)
			sum.OutsideBytes = max(sum.OutsideBytes, s.OutsideBytes)
		}
		fmt.Fprintf(w, "%s: %s%s: %s\n", time.Since(start).Round(time.Second), name, done, formatPackProgress(sum))
	}

	// Start each shard.
	logs := packmove.ShardLogPaths(cfg.LogPath, n)
	for i := range n {
		shard := cfg
		shard.KeyStart, shard.KeyEnd, shard.LogPath = bounds[i], bounds[i+1], logs[i]
		shard.Progress = func(p packmove.PackProgress) { report(i, p, "") }
		g.Go(func() error {
			p, err := pack(ctx, shard)
			report(i, p, fmt.Sprintf(" shard %d done", i))
			return err
		})
	}
}

// firstVolumeBlock returns the first block key of the volume, relative to the
// block prefix.
func firstVolumeBlock(ctx context.Context, vol kvtx.Store, prefix []byte) ([]byte, error) {
	// Read the first key under the block prefix.
	tx, err := vol.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(nil); err != nil {
		return nil, err
	}
	if !it.Valid() {
		return nil, errors.New("the volume holds no blocks")
	}
	return bytes.Clone(it.Key()[len(prefix):]), it.Err()
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
