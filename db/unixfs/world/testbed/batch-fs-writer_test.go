package unixfs_world_testbed

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

type batchMetricRecorder struct {
	metrics []unixfs_world.BatchFSWriterMetric
}

func (r *batchMetricRecorder) RecordBatchFSWriterMetric(metric unixfs_world.BatchFSWriterMetric) {
	r.metrics = append(r.metrics, metric)
}

// TestBatchFSWriter_AddFileBuildsBlob verifies that AddFile accumulates
// entries without mutating the parent directory, and that a subsequent
// Commit flushes the flat batch into the root directory under a single
// world transaction.
func TestBatchFSWriter_AddFileBuildsBlob(t *testing.T) {
	// Prepare the logger and recorder for batch-writer metrics.
	ctx := context.Background()
	recorder := &batchMetricRecorder{}
	ctx = unixfs_world.WithBatchFSWriterMetricsRecorder(ctx, recorder)
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the batch writer.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the batch writer.
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the filesystem to observe batch commits.
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Bind the batch writer to the filesystem root and sender.
	sender := wtb.Volume.GetPeerID()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, sender)

	// Ingest the hello.txt content into the pending batch.
	now := time.Now()
	content := []byte("hello batch")
	if err := bw.AddFile(
		ctx,
		nil,
		"hello.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		bytes.NewReader(content),
		0o644,
		now,
	); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// Commit the file batch and release the writer.
	if err := bw.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	bw.Release()

	// Verify that the recorder observed every batch lifecycle stage.
	for _, stage := range []string{
		"ingest-file-start",
		"ingest-file-complete",
		"commit-start",
		"commit-complete",
		"release",
	} {
		if !slices.ContainsFunc(recorder.metrics, func(metric unixfs_world.BatchFSWriterMetric) bool {
			return metric.Stage == stage
		}) {
			t.Fatalf("missing batch metric stage %q: %#v", stage, recorder.metrics)
		}
	}

	// Open hello.txt from the committed filesystem.
	fh, err := fsHandle.Lookup(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("post-commit Lookup: %v", err)
	}
	defer fh.Release()

	// Verify the committed file size.
	size, err := fh.GetSize(ctx)
	if err != nil {
		t.Fatalf("GetSize: %v", err)
	}
	if size != uint64(len(content)) {
		t.Fatalf("expected size %d got %d", len(content), size)
	}

	// Verify the committed hello.txt content.
	buf := make([]byte, len(content))
	n, err := fh.ReadAt(ctx, 0, buf)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(buf[:n], content) {
		t.Fatalf("content mismatch: %q != %q", buf[:n], content)
	}
}

// TestBatchFSWriter_FlatMultiFile exercises iter 5 with several files added
// at the root and committed in a single pass.
func TestBatchFSWriter_FlatMultiFile(t *testing.T) {
	// Prepare the context and logger for a flat file batch.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the flat file batch.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the flat file batch.
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the filesystem to read the flat batch commit.
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Bind the batch writer to the filesystem root and sender.
	sender := wtb.Volume.GetPeerID()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, sender)

	// Ingest the root files into one pending batch.
	now := time.Now()
	files := map[string][]byte{
		"alpha.txt":   []byte("alpha contents"),
		"bravo.txt":   []byte("bravo contents slightly longer"),
		"charlie.bin": bytes.Repeat([]byte{0xAB}, 64),
	}
	for name, body := range files {
		if err := bw.AddFile(
			ctx,
			nil,
			name,
			unixfs.NewFSCursorNodeType_File(),
			int64(len(body)),
			bytes.NewReader(body),
			0o644,
			now,
		); err != nil {
			t.Fatalf("AddFile %s: %v", name, err)
		}
	}

	// Add the root symlink and commit the flat batch.
	if err := bw.AddSymlink(ctx, nil, "alpha.lnk", []string{"alpha.txt"}, false, now); err != nil {
		t.Fatalf("AddSymlink: %v", err)
	}
	if err := bw.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	bw.Release()

	// Verify the committed size and content of every root file.
	for name, body := range files {
		// Open the committed root file for readback.
		fh, err := fsHandle.Lookup(ctx, name)
		if err != nil {
			t.Fatalf("Lookup %s: %v", name, err)
		}

		// Verify the root file size against its ingested body.
		size, err := fh.GetSize(ctx)
		if err != nil {
			fh.Release()
			t.Fatalf("GetSize %s: %v", name, err)
		}
		if size != uint64(len(body)) {
			fh.Release()
			t.Fatalf("%s size expected %d got %d", name, len(body), size)
		}

		// Verify the root file content and release its handle.
		buf := make([]byte, len(body))
		n, err := fh.ReadAt(ctx, 0, buf)
		fh.Release()
		if err != nil {
			t.Fatalf("ReadAt %s: %v", name, err)
		}
		if !bytes.Equal(buf[:n], body) {
			t.Fatalf("%s content mismatch", name)
		}
	}
}

// TestBatchFSWriter_MissingParent exercises iter 9: AddFile under an
// intermediate dir that was neither declared via AddDir nor pre-existing in
// the FSTree fails at Commit rather than silently auto-creating the dir.
func TestBatchFSWriter_MissingParent(t *testing.T) {
	// Prepare the context and logger for a missing batch parent.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the missing-parent batch.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the missing-parent batch.
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the filesystem containing the batch root.
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Bind the batch writer to the filesystem root and sender.
	sender := wtb.Volume.GetPeerID()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, sender)

	// Ingest a file beneath an undeclared ghost directory.
	if err := bw.AddFile(
		ctx,
		[]string{"ghost"},
		"x.txt",
		unixfs.NewFSCursorNodeType_File(),
		0,
		nil,
		0o644,
		time.Now(),
	); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// Verify that the batch commit rejects the missing parent.
	if err := bw.Commit(ctx); err == nil {
		t.Fatal("expected Commit to error on missing parent")
	}
	bw.Release()
}

// TestBatchFSWriter_Overwrite exercises iter 7: a file existing in the
// target FSTree is replaced by a fresh entry carrying new content, without
// a duplicate dirent appearing.
func TestBatchFSWriter_Overwrite(t *testing.T) {
	// Prepare the context and logger for a file overwrite.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the overwrite batch.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the overwrite batch.
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the filesystem to observe the overwrite commit.
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Resolve the sender and filesystem type for both batches.
	sender := wtb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE

	// First commit: write "greeting.txt" with content "v1".
	bw1 := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, fsType, sender)
	now := time.Now()
	if err := bw1.AddFile(
		ctx,
		nil,
		"greeting.txt",
		unixfs.NewFSCursorNodeType_File(),
		2,
		bytes.NewReader([]byte("v1")),
		0o644,
		now,
	); err != nil {
		t.Fatalf("AddFile v1: %v", err)
	}
	if err := bw1.Commit(ctx); err != nil {
		t.Fatalf("Commit v1: %v", err)
	}
	bw1.Release()

	// Second commit: overwrite "greeting.txt" with "v2-longer".
	bw2 := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, fsType, sender)
	updated := []byte("v2-longer")
	if err := bw2.AddFile(
		ctx,
		nil,
		"greeting.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(updated)),
		bytes.NewReader(updated),
		0o644,
		time.Now(),
	); err != nil {
		t.Fatalf("AddFile v2: %v", err)
	}
	if err := bw2.Commit(ctx); err != nil {
		t.Fatalf("Commit v2: %v", err)
	}
	bw2.Release()

	// Open greeting.txt after the overwrite commit.
	fh, err := fsHandle.Lookup(ctx, "greeting.txt")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	defer fh.Release()

	// Verify that the overwrite replaced the file size.
	size, err := fh.GetSize(ctx)
	if err != nil {
		t.Fatalf("GetSize: %v", err)
	}
	if size != uint64(len(updated)) {
		t.Fatalf("size got %d want %d", size, len(updated))
	}

	// Verify that the overwrite replaced the file content.
	buf := make([]byte, len(updated))
	n, err := fh.ReadAt(ctx, 0, buf)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(buf[:n], updated) {
		t.Fatalf("content got %q want %q", buf[:n], updated)
	}
}

// TestBatchFSWriter_NestedDirs exercises iter 6: multi-directory Commit with
// intermediate dirs declared via AddDir and files landing under them.
func TestBatchFSWriter_NestedDirs(t *testing.T) {
	// Prepare the context and logger for a nested directory batch.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the nested directory batch.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the nested directory batch.
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the filesystem to observe the nested batch commit.
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Bind the batch writer to the filesystem root and sender.
	sender := wtb.Volume.GetPeerID()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, sender)
	now := time.Now()

	// Add files first, in shuffled order, to verify Commit tolerates
	// any-order adds and sorts parents by depth internally.
	if err := bw.AddFile(
		ctx,
		[]string{"docs", "notes"},
		"readme.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len("nested")),
		bytes.NewReader([]byte("nested")),
		0o644,
		now,
	); err != nil {
		t.Fatalf("AddFile readme: %v", err)
	}
	if err := bw.AddFile(
		ctx,
		[]string{"docs"},
		"index.md",
		unixfs.NewFSCursorNodeType_File(),
		int64(len("index")),
		bytes.NewReader([]byte("index")),
		0o644,
		now,
	); err != nil {
		t.Fatalf("AddFile index: %v", err)
	}
	if err := bw.AddDir(ctx, nil, "docs", 0o755, now); err != nil {
		t.Fatalf("AddDir docs: %v", err)
	}
	if err := bw.AddDir(ctx, []string{"docs"}, "notes", 0o755, now); err != nil {
		t.Fatalf("AddDir notes: %v", err)
	}

	// Commit the nested directory batch and release the writer.
	if err := bw.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	bw.Release()

	// Open the nested readme.txt from the committed filesystem.
	fh, _, err := fsHandle.LookupPathPts(ctx, []string{"docs", "notes", "readme.txt"})
	if err != nil {
		t.Fatalf("Lookup nested: %v", err)
	}
	defer fh.Release()

	// Verify the committed nested file size.
	size, err := fh.GetSize(ctx)
	if err != nil {
		t.Fatalf("GetSize nested: %v", err)
	}
	if size != uint64(len("nested")) {
		t.Fatalf("nested size got %d want %d", size, len("nested"))
	}
}

// TestBatchFSWriter_UpdateThenCreate isolates the lost-create defect: a
// create scheduled after an existing-file rewrite inside one Commit must
// converge.
func TestBatchFSWriter_UpdateThenCreate(t *testing.T) {
	// Prepare the context and logger for an update followed by a create.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the update-and-create batch.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the update-and-create batch.
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the filesystem to observe both file commits.
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Resolve the sender and filesystem type for both batches.
	sender := wtb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE

	// Seed the filesystem with the alpha and beta files.
	bw1 := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, fsType, sender)
	now := time.Now()
	for name, body := range map[string]string{"a.txt": "alpha", "b.txt": "beta"} {
		if err := bw1.AddFile(ctx, nil, name, unixfs.NewFSCursorNodeType_File(), int64(len(body)), strings.NewReader(body), 0o644, now); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	if err := bw1.Commit(ctx); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	bw1.Release()

	// Ingest the beta update and the new delta file in one batch.
	bw2 := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, fsType, sender)
	updated := []byte("beta-v2")
	if err := bw2.AddFile(ctx, nil, "b.txt", unixfs.NewFSCursorNodeType_File(), int64(len(updated)), bytes.NewReader(updated), 0o644, now); err != nil {
		t.Fatalf("update b.txt: %v", err)
	}
	created := []byte("delta")
	if err := bw2.AddFile(ctx, nil, "d.txt", unixfs.NewFSCursorNodeType_File(), int64(len(created)), bytes.NewReader(created), 0o644, now); err != nil {
		t.Fatalf("create d.txt: %v", err)
	}
	if err := bw2.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	bw2.Release()

	// Verify that the batch preserved alpha, updated beta and created delta.
	for name, want := range map[string]string{"a.txt": "alpha", "b.txt": "beta-v2", "d.txt": "delta"} {
		// Open the committed file for readback.
		fh, err := fsHandle.Lookup(ctx, name)
		if err != nil {
			t.Fatalf("lookup %s: %v", name, err)
		}

		// Read the committed file content and release its handle.
		size, err := fh.GetSize(ctx)
		if err != nil {
			fh.Release()
			t.Fatalf("size %s: %v", name, err)
		}
		buf := make([]byte, size)
		if _, err := fh.ReadAt(ctx, 0, buf); err != nil {
			fh.Release()
			t.Fatalf("read %s: %v", name, err)
		}
		fh.Release()

		// Verify the committed file content against the expected value.
		if string(buf) != want {
			t.Fatalf("file %s content %q want %q", name, buf, want)
		}
	}
}

func TestBatchFSWriter_DirFileConflictInvalidatesResolvedDir(t *testing.T) {
	// Prepare the context and logger for a directory and file conflict.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the conflicting batch.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the conflicting batch.
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the filesystem containing the conflicting batch root.
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Bind the batch writer to the filesystem root and sender.
	sender := wtb.Volume.GetPeerID()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, sender)

	// Declare a directory and replace it with a pending file.
	now := time.Now()
	if err := bw.AddDir(ctx, nil, "conflict", 0o755, now); err != nil {
		t.Fatalf("AddDir conflict: %v", err)
	}
	if err := bw.AddFile(
		ctx,
		nil,
		"conflict",
		unixfs.NewFSCursorNodeType_File(),
		int64(len("file")),
		bytes.NewReader([]byte("file")),
		0o644,
		now,
	); err != nil {
		t.Fatalf("AddFile conflict: %v", err)
	}

	// Ingest a child file beneath the replaced directory.
	if err := bw.AddFile(
		ctx,
		[]string{"conflict"},
		"child.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len("child")),
		bytes.NewReader([]byte("child")),
		0o644,
		now,
	); err != nil {
		t.Fatalf("AddFile child: %v", err)
	}

	// Verify that the batch commit rejects the replaced parent directory.
	if err := bw.Commit(ctx); err == nil {
		t.Fatal("Commit succeeded after replacing a pending directory with a file")
	}
	bw.Release()
}
