//go:build !js

package spacewave_cli

import (
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// FsRestoreArgs selects an exported World root and its S3 packfiles.
type FsRestoreArgs struct {
	// snapshotPath names the decrypted recovery root file.
	snapshotPath string
	// endpoint is the S3 hostname.
	endpoint string
	// region is the signing region.
	region string
	// bucket holds the packfiles.
	bucket string
	// prefix is the block-store prefix, including the Space ID.
	prefix string
	// objectKey is the UnixFS object to restore.
	objectKey string
}

// BuildFlags builds the recovery location flags.
func (a *FsRestoreArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "snapshot", Usage: "decrypted export-root file (contains secrets)", Required: true, Destination: &a.snapshotPath},
		&cli.StringFlag{Name: "endpoint", Usage: "S3 endpoint hostname", Required: true, Destination: &a.endpoint},
		&cli.StringFlag{Name: "region", Usage: "S3 signing region", Required: true, Destination: &a.region},
		&cli.StringFlag{Name: "bucket", Usage: "S3 bucket", Required: true, Destination: &a.bucket},
		&cli.StringFlag{Name: "prefix", Usage: "block-store prefix containing packs/ and entries/", Required: true, Destination: &a.prefix},
		&cli.StringFlag{Name: "object", Usage: "UnixFS object key in the snapshot", Required: true, Destination: &a.objectKey},
	}
}

// Run restores into a new directory without the original daemon or account state.
func (a *FsRestoreArgs) Run(c *cli.Context) error {
	// Refuse an existing destination so recovery cannot overwrite live data.
	if c.NArg() != 1 {
		return errors.New("one new destination directory is required")
	}
	destination := c.Args().First()
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		if err != nil {
			return err
		}
		return errors.New("restore destination already exists; choose a new directory")
	}

	// Read the recovery root that names the World to restore.
	data, err := os.ReadFile(a.snapshotPath)
	if err != nil {
		return err
	}
	snapshot := &s4wave_world.WorldRootSnapshot{}
	if err := snapshot.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "read recovery root")
	}

	// Read packfiles directly with the existing S3 client and World reader.
	credentials, err := readS3Credentials()
	if err != nil {
		return err
	}
	client, err := block_store_s3.BuildClient(&block_store_s3.ClientConfig{Endpoint: a.endpoint, Region: a.region, Credentials: credentials})
	if err != nil {
		return err
	}
	ctx := c.Context
	le := logrus.NewEntry(logrus.New())
	store := block_store_s3.NewPackStore(le, client, a.bucket, a.prefix)
	defer store.Close()

	// Open the saved World root and a read transaction on it.
	engine, err := world_block.OpenSnapshot(ctx, le, store, snapshot.GetRootRef())
	if err != nil {
		return err
	}
	defer engine.Close()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Open the UnixFS object to restore; it must be a directory.
	handle, err := unixfs_world.BuildFSFromUnixfsRef(ctx, le, tx, "", &unixfs_world.UnixfsRef{ObjectKey: a.objectKey}, false, false, time.Time{})
	if err != nil {
		return err
	}
	defer handle.Release()
	nodeType, err := handle.GetNodeType(ctx)
	if err != nil {
		return err
	}
	if !nodeType.GetIsDirectory() {
		return errors.New("recovery object must be a directory")
	}

	// Keep partially restored data on failure for inspection; success means every file copied.
	if err := os.Mkdir(destination, 0o700); err != nil {
		return err
	}
	return unixfs_sync.Sync(ctx, destination, handle, unixfs_sync.DeleteMode_DeleteMode_NONE, nil)
}

// newFsRestoreCommand builds the standalone S3 recovery command.
func newFsRestoreCommand() *cli.Command {
	args := &FsRestoreArgs{}
	return &cli.Command{
		Name:        "restore-s3",
		Usage:       "restore a UnixFS snapshot directly from S3 into a new local directory",
		ArgsUsage:   "NEW_DIRECTORY",
		Description: "Needs an export-root file and the original packfiles, but no daemon state.\nRead S3 credentials from AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY,\nor two lines on stdin. Decrypt the recovery file before running this command.",
		Flags:       args.BuildFlags(),
		Action:      args.Run,
	}
}
