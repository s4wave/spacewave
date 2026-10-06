//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"slices"
	"strings"

	"github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// debugPayloadRestoreArgs are the arguments of the offline payload restore.
type debugPayloadRestoreArgs struct {
	// spaceID is the SharedObject whose World references the payload.
	spaceID string
	// filePath is the file whose whole contents the payload carried.
	filePath string
	// hash is the base64 digest of the missing block.
	hash string
	// bucketID names the owning bucket when the Space has several.
	bucketID string
}

// BuildFlags returns the payload restore flags.
func (a *debugPayloadRestoreArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "space",
			Usage:       "SharedObject ID of the Space whose World references the payload",
			Required:    true,
			Destination: &a.spaceID,
		},
		&cli.StringFlag{
			Name:        "file",
			Usage:       "file holding the bytes the payload carried",
			Required:    true,
			Destination: &a.filePath,
		},
		&cli.StringFlag{
			Name:        "hash",
			Usage:       "base64 digest of the missing block, as replay names it",
			Required:    true,
			Destination: &a.hash,
		},
		&cli.StringFlag{
			Name:        "bucket",
			Usage:       "bucket that owns the block, when several are named for the Space",
			Destination: &a.bucketID,
		},
	}
}

// Run restores the payload into the stopped volume and prints its ref.
func (a *debugPayloadRestoreArgs) Run(c *cli.Context) error {
	// Read the volume argument, the expected digest and the file.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	want, err := base64.StdEncoding.DecodeString(a.hash)
	if err != nil {
		return errors.Wrap(err, "decode hash")
	}
	data, err := os.ReadFile(a.filePath)
	if err != nil {
		return err
	}

	// Restore the payload and print where it went.
	path := c.Args().First()
	le := logrus.NewEntry(logrus.New())
	ref, err := restorePayload(c.Context, le, path, a.spaceID, a.bucketID, data, want)
	if err != nil {
		return err
	}
	writeFields(os.Stdout, [][2]string{
		{"Volume", path},
		{"Space", a.spaceID},
		{"Restored", ref.MarshalString()},
	})
	return nil
}

// newDebugPayloadRestoreCommand builds the offline payload restore.
func newDebugPayloadRestoreCommand() *cli.Command {
	args := &debugPayloadRestoreArgs{}
	return &cli.Command{
		Name:      "payload-restore",
		Usage:     "rebuild a lost operation payload from its source file",
		ArgsUsage: "<volume-file>",
		Description: "Rebuilds the blob a file write carried, encodes it with the Space World's " +
			"transform and writes it into a stopped bbolt volume under the Space's bucket, " +
			"only when its digest equals --hash. Blob encoding is deterministic, so the same " +
			"bytes yield the same block. Only a payload of one block, a whole file within the " +
			"raw blob size, is supported. Stop the daemon and keep a copy (cp -c clones it on " +
			"APFS) before running it.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// restorePayload rebuilds the payload block of data with the World transform
// of spaceID, checks its digest against want, and writes it into the volume at
// path owned by bucketID, or by the one bucket named for the Space when
// bucketID is empty.
func restorePayload(ctx context.Context, le *logrus.Entry, path, spaceID, bucketID string, data, want []byte) (*block.BlockRef, error) {
	// Hold the stopped volume exclusively.
	if err := requireVolumeStopped(path); err != nil {
		return nil, err
	}
	vol, err := volume_bolt.NewBolt(ctx, le, &volume_bolt.Config{
		Path:          path,
		NoGenerateKey: true,
		NoWriteKey:    true,
		Exclusive:     true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "open volume")
	}
	defer vol.Close()

	// Find the World transform and the bucket that owns the Space's blocks.
	conf, err := readWorldTransform(volume_bolt.GetBoltDB(vol), spaceID)
	if err != nil {
		return nil, err
	}
	if bucketID == "" {
		bucketID, err = findSpaceBucket(ctx, vol.GetRefGraph(), spaceID)
		if err != nil {
			return nil, err
		}
	}

	// Encode the blob and refuse a block other than the missing one.
	ref, encoded, err := encodePayload(ctx, le, conf, vol.GetHashType(), data)
	if err != nil {
		return nil, err
	}
	if got := ref.GetHash().GetHash(); !bytes.Equal(got, want) {
		return nil, errors.Errorf("rebuilt block digest %s differs from %s", base64.StdEncoding.EncodeToString(got), base64.StdEncoding.EncodeToString(want))
	}

	// Write the block with its bucket ownership.
	le.Infof("restoring %s into bucket %s", ref.MarshalString(), bucketID)
	_, _, err = vol.PrepareOwnedBlock(ctx, bucketID, encoded, &block.PutOpts{
		HashType:      ref.GetHash().GetHashType(),
		ForceBlockRef: ref,
	})
	if err != nil {
		return nil, errors.Wrap(err, "write block")
	}
	return ref, nil
}

// readWorldTransform reads the World transform from the replay cursor the
// Space keeps in its account's object store.
func readWorldTransform(db *bbolt.DB, spaceID string) (*block_transform.Config, error) {
	// Collect the cursor values of the Space across the object stores.
	conf := kvkey.DefaultConfig()
	prefix := slices.Concat(conf.GetPrefix(), conf.GetObjectStorePrefix())
	suffix := []byte("so-local/" + spaceID + "/world-replay/cursor")
	var values [][]byte
	err := db.View(func(tx *bbolt.Tx) error {
		// Skip a volume without the store bucket.
		b := tx.Bucket([]byte("hydra"))
		if b == nil {
			return nil
		}

		// Match the cursor key of the Space in every object store.
		cur := b.Cursor()
		for k, v := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cur.Next() {
			if bytes.HasSuffix(k, suffix) {
				values = append(values, bytes.Clone(v))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, errors.Errorf("found %d replay cursors for space %s, expected one", len(values), spaceID)
	}

	// Take the inline transform of the World the cursor holds.
	cursor := &sobject_world_engine.ReplayCursor{}
	if err := cursor.UnmarshalVT(values[0]); err != nil {
		return nil, errors.Wrap(err, "decode replay cursor")
	}
	state := cursor.GetHead()
	if state == nil {
		state = cursor.GetBase()
	}
	xfrm := state.GetHeadRef().GetTransformConf()
	if xfrm.GetEmpty() {
		return nil, errors.New("replay cursor names no inline World transform")
	}
	return xfrm, nil
}

// findSpaceBucket returns the one bucket under the permanent root whose ID has
// spaceID as a path segment, as the providers' block store buckets do.
func findSpaceBucket(ctx context.Context, rg block_gc.RefGraphOps, spaceID string) (string, error) {
	// List the buckets under the permanent root.
	roots, err := rg.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
	if err != nil {
		return "", err
	}

	// Keep the buckets named for the Space and require exactly one.
	var found []string
	for _, root := range roots {
		id, ok := block_gc.ParseBucketIRI(root)
		if ok && slices.Contains(strings.Split(id, "/"), spaceID) {
			found = append(found, id)
		}
	}
	if len(found) != 1 {
		return "", errors.Errorf("found buckets %v for space %s, expected one", found, spaceID)
	}
	return found[0], nil
}

// encodePayload builds the blob of data as a file write does and returns its
// block ref and stored bytes. A blob of more than one block is refused.
func encodePayload(ctx context.Context, le *logrus.Entry, conf *block_transform.Config, hashType hash.HashType, data []byte) (*block.BlockRef, []byte, error) {
	// Build the World transformer.
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, transform_all.BuildFactorySet(), conf)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build world transform")
	}

	// Write the blob into a scratch store.
	store := block_store_inmem.NewInmemBlock(kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), hashType, false)
	tx, bcs := block.NewTransaction(store, xfrm, nil, nil)
	blb, err := blob.BuildBlobWithBytes(ctx, data, bcs)
	if err != nil {
		return nil, nil, err
	}
	if blb.GetBlobType() != blob.BlobType_BlobType_RAW {
		return nil, nil, errors.Errorf("blob of %d bytes spans several blocks", len(data))
	}
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		return nil, nil, err
	}

	// Read back the stored bytes.
	encoded, found, err := store.GetBlock(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, block.ErrNotFound
	}
	return ref, encoded, nil
}
