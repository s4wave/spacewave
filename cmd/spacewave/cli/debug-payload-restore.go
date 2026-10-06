//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

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
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// restoreBatchBytes bounds the block bytes one source tree restore
// transaction writes.
const restoreBatchBytes = 32 << 20

// debugPayloadRestoreArgs are the arguments of the offline payload restore.
type debugPayloadRestoreArgs struct {
	// spaceID is the SharedObject whose World references the payload.
	spaceID string
	// filePath is the file whose whole contents the payload carried.
	filePath string
	// hash is the base64 digest of the missing block.
	hash string
	// sourceRoot is a tree whose files the payloads carried.
	sourceRoot string
	// bucketID names the owning bucket when the Space has several.
	bucketID string
	// dryRun reports the missing blocks without writing them.
	dryRun bool
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
			Usage:       "file holding the bytes the payload carried, with --hash",
			Destination: &a.filePath,
		},
		&cli.StringFlag{
			Name:        "hash",
			Usage:       "base64 digest of the missing block, as replay names it",
			Destination: &a.hash,
		},
		&cli.StringFlag{
			Name:        "source-root",
			Usage:       "restore the missing whole-file payloads of every file under this directory",
			Destination: &a.sourceRoot,
		},
		&cli.StringFlag{
			Name:        "bucket",
			Usage:       "bucket that owns the block, when several are named for the Space",
			Destination: &a.bucketID,
		},
		&cli.BoolFlag{
			Name:        "dry-run",
			Usage:       "with --source-root, report the missing blocks without writing them",
			Destination: &a.dryRun,
		},
	}
}

// Run restores the payloads into the stopped volume and reports them.
func (a *debugPayloadRestoreArgs) Run(c *cli.Context) error {
	// Read the volume argument and choose the mode.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	path := c.Args().First()
	le := logrus.NewEntry(logrus.New())
	if a.sourceRoot != "" {
		if a.filePath != "" || a.hash != "" {
			return errors.New("--source-root excludes --file and --hash")
		}
		return a.runSourceRoot(c.Context, le, path)
	}
	if a.filePath == "" || a.hash == "" {
		return errors.New("expected --file and --hash, or --source-root")
	}

	// Read the expected digest and the file.
	want, err := base64.StdEncoding.DecodeString(a.hash)
	if err != nil {
		return errors.Wrap(err, "decode hash")
	}
	data, err := os.ReadFile(a.filePath)
	if err != nil {
		return err
	}

	// Restore the payload and print what was written.
	ref, total, restored, err := restorePayload(c.Context, le, path, a.spaceID, a.bucketID, data, want)
	if err != nil {
		return err
	}
	writeFields(os.Stdout, [][2]string{
		{"Volume", path},
		{"Space", a.spaceID},
		{"Payload", ref.MarshalString()},
		{"Present blocks", strconv.Itoa(total - restored)},
		{"Restored blocks", strconv.Itoa(restored)},
	})
	return nil
}

// runSourceRoot restores the missing payloads of the source tree and prints
// the counts.
func (a *debugPayloadRestoreArgs) runSourceRoot(ctx context.Context, le *logrus.Entry, path string) error {
	// Walk the tree, then label the missing blocks by whether they were
	// written.
	res, err := restoreSourceTree(ctx, le, path, a.spaceID, a.bucketID, a.sourceRoot, a.dryRun)
	if err != nil {
		return err
	}
	restored := "Restored blocks"
	if a.dryRun {
		restored = "Missing blocks"
	}
	writeFields(os.Stdout, [][2]string{
		{"Volume", path},
		{"Space", a.spaceID},
		{"Files", strconv.Itoa(res.files)},
		{"Present blocks", strconv.Itoa(res.present)},
		{restored, strconv.Itoa(res.restored)},
		{"Restored bytes", strconv.FormatInt(res.restoredBytes, 10)},
		{"Multi-extent files", strconv.Itoa(len(res.extentFiles))},
	})
	return nil
}

// newDebugPayloadRestoreCommand builds the offline payload restore.
func newDebugPayloadRestoreCommand() *cli.Command {
	args := &debugPayloadRestoreArgs{}
	return &cli.Command{
		Name:      "payload-restore",
		Usage:     "rebuild lost operation payloads from their source files",
		ArgsUsage: "<volume-file>",
		Description: "Rebuilds the blob a file write carried, encodes it with the Space World's " +
			"transform and writes its blocks into a stopped bbolt volume under the Space's " +
			"bucket. Blob encoding and chunking are deterministic, so the same bytes yield the " +
			"same blocks. With --file, the blob is written only when its root digest equals " +
			"--hash. With --source-root, the whole-file blob of every regular file under the " +
			"directory is rebuilt and each block the volume lacks is written; blocks no " +
			"operation references stay owned by the bucket until a repair removes them. A " +
			"file larger than one write extent may have been written in several payloads, " +
			"which the walk does not rebuild; it logs those files. Stop the daemon and keep a " +
			"copy (cp -c clones it on APFS) before running it.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// restorePayload rebuilds the payload blob of data with the World transform
// of spaceID, checks its root digest against want, and writes the blocks the
// volume at path lacks, owned by bucketID, or by the one bucket named for the
// Space when bucketID is empty. It returns the root ref, the blob's block
// count, and the number of blocks written.
func restorePayload(ctx context.Context, le *logrus.Entry, path, spaceID, bucketID string, data, want []byte) (*block.BlockRef, int, int, error) {
	// Open the stopped volume with the World transform and owning bucket.
	rv, err := openRestoreVolume(ctx, le, path, spaceID, bucketID)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rv.vol.Close()

	// Encode the blob and refuse a root other than the missing one.
	ref, entries, err := encodePayload(ctx, rv.xfrm, rv.vol.GetHashType(), data)
	if err != nil {
		return nil, 0, 0, err
	}
	if got := ref.GetHash().GetHash(); !bytes.Equal(got, want) {
		return nil, 0, 0, errors.Errorf("rebuilt block digest %s differs from %s", base64.StdEncoding.EncodeToString(got), base64.StdEncoding.EncodeToString(want))
	}

	// Write the missing blocks with their bucket ownership.
	missing, err := missingBlocks(ctx, rv.vol, entries)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(missing) == 0 {
		return ref, len(entries), 0, nil
	}
	le.Infof("restoring %d of %d blocks of %s into bucket %s", len(missing), len(entries), ref.MarshalString(), rv.bucketID)
	if err := rv.vol.PrepareOwnedBlockBatch(ctx, rv.bucketID, missing); err != nil {
		return nil, 0, 0, errors.Wrap(err, "write blocks")
	}
	return ref, len(entries), len(missing), nil
}

// missingBlocks returns the entries whose blocks vol lacks.
func missingBlocks(ctx context.Context, vol *volume_bolt.Bolt, entries []*block.PutBatchEntry) ([]*block.PutBatchEntry, error) {
	// Probe the blocks in one batch.
	refs := make([]*block.BlockRef, len(entries))
	for i, e := range entries {
		refs[i] = e.Ref
	}
	exists, err := vol.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, err
	}

	// Keep the entries not found.
	var missing []*block.PutBatchEntry
	for i, e := range entries {
		if !exists[i] {
			missing = append(missing, e)
		}
	}
	return missing, nil
}

// sourceTreeResult counts the outcome of a source tree restore.
type sourceTreeResult struct {
	// files is the number of regular files walked.
	files int
	// present is the number of distinct blocks the volume already held.
	present int
	// restored is the number of distinct blocks written, or missing in a
	// dry run.
	restored int
	// restoredBytes is the encoded size of the restored blocks.
	restoredBytes int64
	// extentFiles are the files larger than one write extent.
	extentFiles []string
}

// restoreSourceTree rebuilds the whole-file payload blob of every regular
// file under root and writes each block the volume at path lacks, owned by
// the Space's bucket. A dry run only counts the missing blocks.
func restoreSourceTree(ctx context.Context, le *logrus.Entry, path, spaceID, bucketID, root string, dryRun bool) (*sourceTreeResult, error) {
	// Open the stopped volume with the World transform and owning bucket.
	rv, err := openRestoreVolume(ctx, le, path, spaceID, bucketID)
	if err != nil {
		return nil, err
	}
	defer rv.vol.Close()

	// Write the pending blocks in bounded transactions.
	res := &sourceTreeResult{}
	var pending []*block.PutBatchEntry
	var pendingBytes int
	flush := func() error {
		if dryRun || len(pending) == 0 {
			return nil
		}
		if err := rv.vol.PrepareOwnedBlockBatch(ctx, rv.bucketID, pending); err != nil {
			return errors.Wrap(err, "write blocks")
		}
		pending, pendingBytes = nil, 0
		return nil
	}

	// Hold the source tree so the walk cannot leave it.
	src, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	srcFS := src.FS()

	// Rebuild each regular file's blob and queue the blocks the volume lacks.
	seen := make(map[string]struct{})
	err = fs.WalkDir(srcFS, ".", func(filePath string, d fs.DirEntry, err error) error {
		// Read each regular file and note one too large for a single write.
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(srcFS, filePath)
		if err != nil {
			return err
		}
		res.files++
		if len(data) > unixfs_block_fs.OptimalWriteSize {
			res.extentFiles = append(res.extentFiles, filePath)
			le.Warnf("%s is larger than one write extent; restoring its whole-file blob only", filePath)
		}
		_, entries, err := encodePayload(ctx, rv.xfrm, rv.vol.GetHashType(), data)
		if err != nil {
			return errors.Wrap(err, filePath)
		}

		// Skip the blocks already handled and probe the rest.
		entries = slices.DeleteFunc(entries, func(e *block.PutBatchEntry) bool {
			key := e.Ref.MarshalString()
			_, dup := seen[key]
			seen[key] = struct{}{}
			return dup
		})
		missing, err := missingBlocks(ctx, rv.vol, entries)
		if err != nil {
			return err
		}
		res.present += len(entries) - len(missing)

		// Queue the missing blocks and write them once the batch is full.
		for _, e := range missing {
			le.Infof("missing %s from %s", e.Ref.MarshalString(), filePath)
			res.restored++
			res.restoredBytes += int64(len(e.Data))
			if !dryRun {
				pending = append(pending, e)
				pendingBytes += len(e.Data)
			}
		}
		if pendingBytes < restoreBatchBytes {
			return nil
		}
		return flush()
	})
	if err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return res, nil
}

// restoreVolume is a stopped volume opened for payload restores.
type restoreVolume struct {
	// vol is the exclusively held volume.
	vol *volume_bolt.Bolt
	// xfrm is the Space World's block transformer.
	xfrm block.Transformer
	// bucketID is the bucket that owns the Space's blocks.
	bucketID string
}

// openRestoreVolume holds the stopped volume at path exclusively and resolves
// the World transform and owning bucket of spaceID. An empty bucketID selects
// the one bucket named for the Space. The caller closes the volume.
func openRestoreVolume(ctx context.Context, le *logrus.Entry, path, spaceID, bucketID string) (*restoreVolume, error) {
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

	// Build the World transformer and find the bucket that owns the Space's
	// blocks.
	rv, err := resolveRestoreVolume(ctx, le, vol, spaceID, bucketID)
	if err != nil {
		vol.Close()
		return nil, err
	}
	return rv, nil
}

// resolveRestoreVolume resolves the World transformer and owning bucket of
// spaceID in vol.
func resolveRestoreVolume(ctx context.Context, le *logrus.Entry, vol *volume_bolt.Bolt, spaceID, bucketID string) (*restoreVolume, error) {
	// Build the World transformer from the replay cursor.
	conf, err := readWorldTransform(volume_bolt.GetBoltDB(vol), spaceID)
	if err != nil {
		return nil, err
	}
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, transform_all.BuildFactorySet(), conf)
	if err != nil {
		return nil, errors.Wrap(err, "build world transform")
	}

	// Find the bucket named for the Space unless one is given.
	if bucketID == "" {
		bucketID, err = findSpaceBucket(ctx, vol.GetRefGraph(), spaceID)
		if err != nil {
			return nil, err
		}
	}
	return &restoreVolume{vol: vol, xfrm: xfrm, bucketID: bucketID}, nil
}

// readWorldTransform reads the World transform from the replay cursor the
// Space keeps in its account's object store.
func readWorldTransform(db *bbolt.DB, spaceID string) (*block_transform.Config, error) {
	// Collect the cursor values of the Space across the object stores. The
	// local provider keeps local state under so/<id>/ls/, the Spacewave
	// provider under so-local/<id>/.
	conf := kvkey.DefaultConfig()
	prefix := slices.Concat(conf.GetPrefix(), conf.GetObjectStorePrefix())
	suffixes := [][]byte{
		[]byte("so/" + spaceID + "/ls/world-replay/cursor"),
		[]byte("so-local/" + spaceID + "/world-replay/cursor"),
	}
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
			if slices.ContainsFunc(suffixes, func(suffix []byte) bool { return bytes.HasSuffix(k, suffix) }) {
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
// root ref and every block it stored, encoded with xfrm.
func encodePayload(ctx context.Context, xfrm block.Transformer, hashType hash.HashType, data []byte) (*block.BlockRef, []*block.PutBatchEntry, error) {
	// Write the blob into a scratch store that records each block.
	store := &recordStore{
		StoreOps: block_store_inmem.NewInmemBlock(kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), hashType, false),
		seen:     make(map[string]struct{}),
	}
	tx, bcs := block.NewTransaction(store, xfrm, nil, nil)
	if _, err := blob.BuildBlobWithBytes(ctx, data, bcs); err != nil {
		return nil, nil, err
	}
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		return nil, nil, err
	}
	return ref, store.entries, nil
}

// recordStore records the distinct blocks written through it.
type recordStore struct {
	block.StoreOps

	// mtx guards the fields below; transactions write blocks concurrently.
	mtx sync.Mutex
	// seen holds the marshaled refs of the recorded blocks.
	seen map[string]struct{}
	// entries are the recorded blocks in write order.
	entries []*block.PutBatchEntry
}

// PutBlock writes and records a block.
func (s *recordStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	s.record(&block.PutBatchEntry{Ref: ref, Data: data})
	return ref, existed, nil
}

// PutBlockBatch writes and records a batch of blocks.
func (s *recordStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if err := s.StoreOps.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Tombstone {
			s.record(e)
		}
	}
	return nil
}

// record keeps a copy of e unless its block is already recorded.
func (s *recordStore) record(e *block.PutBatchEntry) {
	// Skip a block already recorded.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	key := e.Ref.MarshalString()
	if _, ok := s.seen[key]; ok {
		return
	}

	// Copy the block, which the caller may reuse.
	s.seen[key] = struct{}{}
	s.entries = append(s.entries, &block.PutBatchEntry{
		Ref:  e.Ref.Clone(),
		Data: bytes.Clone(e.Data),
		Refs: e.Refs,
	})
}
