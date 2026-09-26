package unixfs_world_testbed

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/prng"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// batchCountBucket counts the block batches written to the wrapped bucket.
type batchCountBucket struct {
	bucket.BucketOps

	mu      sync.Mutex
	batches int
}

// PutBlockBatch counts the batch and forwards it.
func (b *batchCountBucket) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	b.mu.Lock()
	b.batches++
	b.mu.Unlock()
	return b.BucketOps.PutBlockBatch(ctx, entries)
}

// count returns the number of batches written so far.
func (b *batchCountBucket) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.batches
}

// TestBatchFSWriter_CommitWritesBlobsWithTree checks that AddFile keeps a
// chunked blob out of the bucket and Commit writes it in the same block batch
// as the tree that references it.
func TestBatchFSWriter_CommitWritesBlobsWithTree(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer htb.Release()

	// Run a world engine over a bucket that counts block batches.
	base, err := htb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()
	counter := &batchCountBucket{BucketOps: base.GetBucket()}
	cursor := bucket_lookup.NewCursor(
		ctx,
		htb.Bus,
		le,
		htb.StepFactorySet,
		counter,
		base.GetTransformer(),
		base.GetRef(),
		base.GetOpArgs(),
		base.GetTransformConf(),
	)
	defer cursor.Release()
	eng, err := world_block.NewEngine(ctx, le, cursor, unixfs_world.LookupFsOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	ws := world.NewEngineWorldState(eng, true)

	now := time.Now()
	sender := htb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE
	if _, _, err := unixfs_world.FsInit(ctx, ws, sender, objKey, fsType, nil, true, now); err != nil {
		t.Fatal(err)
	}

	// A 1 MiB file spans several chunks, so the blob stages chunk writes.
	content := make([]byte, 1<<20)
	if _, err := io.ReadFull(prng.BuildSeededReader([]byte("batch drain")), content); err != nil {
		t.Fatal(err)
	}
	bw := unixfs_world.NewBatchFSWriter(ws, objKey, fsType, sender)
	defer bw.Release()
	before := counter.count()
	if err := bw.AddFile(
		ctx,
		nil,
		"large.bin",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		bytes.NewReader(content),
		0o644,
		now,
	); err != nil {
		t.Fatal(err)
	}
	if got := counter.count() - before; got != 0 {
		t.Fatalf("AddFile batches: got %d want 0", got)
	}

	// The tree write carries the blob; the engine's publication is the other.
	if err := bw.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got := counter.count() - before; got != 2 {
		t.Fatalf("Commit batches: got %d want 2", got)
	}

	// The committed file reads back from the bucket.
	rootCursor, err := unixfs_world.FollowUnixfsRef(ctx, le, ws, &unixfs_world.UnixfsRef{ObjectKey: objKey}, sender, false)
	if err != nil {
		t.Fatal(err)
	}
	fsHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		t.Fatal(err)
	}
	defer fsHandle.Release()
	fh, err := fsHandle.Lookup(ctx, "large.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Release()
	got := make([]byte, len(content))
	n, err := fh.ReadAt(ctx, 0, got)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:n], content) {
		t.Fatal("committed file contents differ")
	}
}
