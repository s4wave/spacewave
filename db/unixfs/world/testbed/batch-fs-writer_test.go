package unixfs_world_testbed

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/file"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
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

// commitMergedHook calls its function with the attempt number each time a
// Commit finishes merging, before the merge is published.
type commitMergedHook func(attempt int)

func (h commitMergedHook) RecordBatchFSWriterMetric(metric unixfs_world.BatchFSWriterMetric) {
	if metric.Stage == "commit-merged" {
		h(metric.Attempt)
	}
}

// newRaceTestbed starts a World engine with an initialized filesystem at
// objKey and returns its World testbed and filesystem handle.
func newRaceTestbed(ctx context.Context, t *testing.T) (*world_testbed.Testbed, *unixfs.FSHandle) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(fsHandle.Release)
	return wtb, fsHandle
}

// addRaceFile adds a root file holding content to bw.
func addRaceFile(ctx context.Context, t *testing.T, bw *unixfs_world.BatchFSWriter, name, content string) {
	t.Helper()
	if err := bw.AddFile(
		ctx,
		nil,
		name,
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		strings.NewReader(content),
		0o644,
		time.Now(),
	); err != nil {
		t.Fatalf("AddFile %s: %v", name, err)
	}
}

// commitRaceFiles commits the root files in files as a second writer.
func commitRaceFiles(ctx context.Context, t *testing.T, wtb *world_testbed.Testbed, files map[string]string) {
	t.Helper()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID())
	defer bw.Release()
	for name, content := range files {
		addRaceFile(ctx, t, bw, name, content)
	}
	if err := bw.Commit(ctx); err != nil {
		t.Errorf("second writer Commit: %v", err)
	}
}

// readRaceFile returns the content of a root file and whether it exists.
func readRaceFile(ctx context.Context, t *testing.T, fsHandle *unixfs.FSHandle, name string) (string, bool) {
	t.Helper()
	fh, err := fsHandle.Lookup(ctx, name)
	if errors.Is(err, unixfs_errors.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatalf("Lookup %s: %v", name, err)
	}
	defer fh.Release()
	size, err := fh.GetSize(ctx)
	if err != nil {
		t.Fatalf("GetSize %s: %v", name, err)
	}
	buf := make([]byte, size)
	n, err := fh.ReadAt(ctx, 0, buf)
	if err != nil {
		t.Fatalf("ReadAt %s: %v", name, err)
	}
	return string(buf[:n]), true
}

// TestBatchFSWriter_RaceConflictRefuses has a second writer create a path the
// upload expects missing, just before each of the upload's four attempts, with
// unrelated writes losing the races before it. The upload must refuse with a
// FileConflictError and leave the other writer's bytes in place.
func TestBatchFSWriter_RaceConflictRefuses(t *testing.T) {
	for k := 1; k <= 4; k++ {
		t.Run(fmt.Sprintf("attempt-%d", k), func(t *testing.T) {
			// Start a filesystem and an upload that expects conflict.txt missing.
			ctx := context.Background()
			wtb, fsHandle := newRaceTestbed(ctx, t)
			bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID())
			defer bw.Release()
			if err := bw.ExpectMissing(nil, "conflict.txt"); err != nil {
				t.Fatal(err)
			}
			addRaceFile(ctx, t, bw, "conflict.txt", "upload")
			addRaceFile(ctx, t, bw, "upload.txt", "upload")

			// Make attempt k see the conflict and each earlier attempt lose a race.
			second := func(attempt int) {
				if attempt+1 == k {
					commitRaceFiles(ctx, t, wtb, map[string]string{"conflict.txt": "other"})
					return
				}
				commitRaceFiles(ctx, t, wtb, map[string]string{fmt.Sprintf("other-%d.txt", attempt): "other"})
			}
			if k == 1 {
				second(0)
			}
			var last int
			hook := commitMergedHook(func(attempt int) {
				last = attempt
				if attempt < k {
					second(attempt)
				}
			})

			// The upload refuses with the conflicting path.
			err := bw.Commit(unixfs_world.WithBatchFSWriterMetricsRecorder(ctx, hook))
			var conflict *unixfs_world.FileConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("Commit: want FileConflictError, got %v", err)
			}
			if last != k-1 {
				t.Fatalf("merged %d attempts before the conflict, want %d", last, k-1)
			}

			// The other writer's bytes remain and none of the upload's were written.
			if got, _ := readRaceFile(ctx, t, fsHandle, "conflict.txt"); got != "other" {
				t.Fatalf("conflict.txt holds %q, want the other writer's bytes", got)
			}
			if _, ok := readRaceFile(ctx, t, fsHandle, "upload.txt"); ok {
				t.Fatal("upload.txt was written by a refused upload")
			}
		})
	}
}

// TestBatchFSWriter_RaceUnrelatedPublishes has a second writer write unrelated
// paths after each of the upload's three staged attempts, so the upload loses
// every race and publishes on its fourth attempt, under the writer. Both
// writers' files must read back.
func TestBatchFSWriter_RaceUnrelatedPublishes(t *testing.T) {
	// Start a filesystem and an upload of two files.
	ctx := context.Background()
	wtb, fsHandle := newRaceTestbed(ctx, t)
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID())
	defer bw.Release()
	addRaceFile(ctx, t, bw, "upload-a.txt", "a")
	addRaceFile(ctx, t, bw, "upload-b.txt", "b")

	// Lose the first three races to unrelated writes.
	var last int
	hook := commitMergedHook(func(attempt int) {
		last = attempt
		if attempt <= 3 {
			commitRaceFiles(ctx, t, wtb, map[string]string{fmt.Sprintf("other-%d.txt", attempt): "other"})
		}
	})
	if err := bw.Commit(unixfs_world.WithBatchFSWriterMetricsRecorder(ctx, hook)); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if last != 4 {
		t.Fatalf("published on attempt %d, want 4", last)
	}

	// Both writers' files read back.
	want := map[string]string{
		"upload-a.txt": "a",
		"upload-b.txt": "b",
		"other-1.txt":  "other",
		"other-2.txt":  "other",
		"other-3.txt":  "other",
	}
	for name, content := range want {
		if got, ok := readRaceFile(ctx, t, fsHandle, name); !ok || got != content {
			t.Fatalf("%s holds %q (exists %v), want %q", name, got, ok, content)
		}
	}
}

// TestBatchFSWriter_CancelledCommitLeavesRoot cancels a Commit after its merge
// and before its publication; the filesystem root must not change.
func TestBatchFSWriter_CancelledCommitLeavesRoot(t *testing.T) {
	// Start a filesystem and an upload, and record the root before it.
	ctx := context.Background()
	wtb, fsHandle := newRaceTestbed(ctx, t)
	before := readRootRef(ctx, t, wtb.WorldState, objKey)
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID())
	defer bw.Release()
	addRaceFile(ctx, t, bw, "upload.txt", "upload")

	// Cancel the commit once its first merge finishes.
	commitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	hook := commitMergedHook(func(int) { cancel() })
	if err := bw.Commit(unixfs_world.WithBatchFSWriterMetricsRecorder(commitCtx, hook)); err == nil {
		t.Fatal("cancelled Commit succeeded")
	}

	// The root is unchanged and the upload's file is absent.
	if after := readRootRef(ctx, t, wtb.WorldState, objKey); !after.EqualsRef(before) {
		t.Fatalf("root changed from %v to %v", before, after)
	}
	if _, ok := readRaceFile(ctx, t, fsHandle, "upload.txt"); ok {
		t.Fatal("upload.txt was written by a cancelled Commit")
	}
}

// stageRootCounter counts the Volume's stage roots when a Commit publishes,
// before the writer releases its stage.
type stageRootCounter struct {
	ctx   context.Context
	t     *testing.T
	wtb   *world_testbed.Testbed
	roots int
}

func (c *stageRootCounter) RecordBatchFSWriterMetric(metric unixfs_world.BatchFSWriterMetric) {
	if metric.Stage != "commit-complete" {
		return
	}
	err := c.wtb.Volume.ListRoots(c.ctx, "stage:", func(block.NamedRoot) error {
		c.roots++
		return nil
	})
	if err != nil {
		c.t.Errorf("ListRoots: %v", err)
	}
}

// TestBatchFSWriter_StageHoldsTreeTop uploads 30,000 files in 300 directories
// and counts the stage roots left when the upload publishes: once with every
// file small enough for its file node to embed, which builds no blob, and once
// with every tenth file over that size, whose blob its directory's put names
// through the stage that holds it. Either way the stage keeps only the top of
// the tree for its release, not a root per file.
func TestBatchFSWriter_StageHoldsTreeTop(t *testing.T) {
	for _, tc := range []struct {
		name string
		// largeEvery makes every largeEvery-th file larger than
		// file.MaxInlineRawBlobSize, or none when zero.
		largeEvery int
	}{
		{name: "small"},
		{name: "mixed", largeEvery: 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testStageHoldsTreeTop(t, tc.largeEvery)
		})
	}
}

// testStageHoldsTreeTop runs TestBatchFSWriter_StageHoldsTreeTop with every
// largeEvery-th file over file.MaxInlineRawBlobSize.
func testStageHoldsTreeTop(t *testing.T, largeEvery int) {
	// Start a filesystem and an upload of 30,000 files.
	const n, perDir = 30000, 100
	ctx := context.Background()
	wtb, fsHandle := newRaceTestbed(ctx, t)
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID())
	defer bw.Release()
	now := time.Now()
	content := func(i int) string {
		c := fmt.Sprintf("file %d", i)
		if largeEvery != 0 && i%largeEvery == 0 {
			c += strings.Repeat(".", file.MaxInlineRawBlobSize)
		}
		return c
	}
	for i := range n {
		dir := fmt.Sprintf("d%d", i/perDir)
		if i%perDir == 0 {
			if err := bw.AddDir(ctx, nil, dir, 0o755, now); err != nil {
				t.Fatal(err)
			}
		}
		c := content(i)
		if err := bw.AddFile(ctx, []string{dir}, fmt.Sprintf("f%d", i), unixfs.NewFSCursorNodeType_File(), int64(len(c)), strings.NewReader(c), 0o644, now); err != nil {
			t.Fatal(err)
		}
	}

	// Publish, counting the stage roots before the stage is released.
	counter := &stageRootCounter{ctx: ctx, t: t, wtb: wtb}
	if err := bw.Commit(unixfs_world.WithBatchFSWriterMetricsRecorder(ctx, counter)); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if counter.roots >= 1000 {
		t.Fatalf("stage holds %d roots at release for %d files, want under 1,000", counter.roots, n)
	}
	t.Logf("stage holds %d roots at release for %d files", counter.roots, n)

	// The last small file and the last large one read back through their
	// directories.
	for _, i := range []int{n - 1, n - largeEvery} {
		if i >= n {
			continue
		}
		dir, err := fsHandle.Lookup(ctx, fmt.Sprintf("d%d", i/perDir))
		if err != nil {
			t.Fatal(err)
		}
		fh, err := dir.Lookup(ctx, fmt.Sprintf("f%d", i))
		dir.Release()
		if err != nil {
			t.Fatal(err)
		}
		want := content(i)
		buf := make([]byte, len(want)+1)
		got, err := fh.ReadAt(ctx, 0, buf)
		fh.Release()
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if string(buf[:got]) != want {
			t.Fatalf("file %d holds %d bytes, want %d", i, got, len(want))
		}
	}
}
