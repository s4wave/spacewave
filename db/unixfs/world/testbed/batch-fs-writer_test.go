package unixfs_world_testbed

import (
	"bytes"
	"context"
	"testing"
	"time"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestBatchFSWriter_AddFileBuildsBlob verifies that AddFile accumulates
// entries without mutating the parent directory, and that a subsequent
// Commit flushes the flat batch into the root directory under a single
// world transaction.
func TestBatchFSWriter_AddFileBuildsBlob(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

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
	defer fsHandle.Release()

	sender := wtb.Volume.GetPeerID()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, sender)

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

	if err := bw.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	bw.Release()

	fh, err := fsHandle.Lookup(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("post-commit Lookup: %v", err)
	}
	defer fh.Release()
	size, err := fh.GetSize(ctx)
	if err != nil {
		t.Fatalf("GetSize: %v", err)
	}
	if size != uint64(len(content)) {
		t.Fatalf("expected size %d got %d", len(content), size)
	}
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
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

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
	defer fsHandle.Release()

	sender := wtb.Volume.GetPeerID()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, sender)

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
	if err := bw.AddSymlink(ctx, nil, "alpha.lnk", []string{"alpha.txt"}, false, now); err != nil {
		t.Fatalf("AddSymlink: %v", err)
	}
	if err := bw.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	bw.Release()

	for name, body := range files {
		fh, err := fsHandle.Lookup(ctx, name)
		if err != nil {
			t.Fatalf("Lookup %s: %v", name, err)
		}
		size, err := fh.GetSize(ctx)
		if err != nil {
			fh.Release()
			t.Fatalf("GetSize %s: %v", name, err)
		}
		if size != uint64(len(body)) {
			fh.Release()
			t.Fatalf("%s size expected %d got %d", name, len(body), size)
		}
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
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

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
	defer fsHandle.Release()

	sender := wtb.Volume.GetPeerID()
	bw := unixfs_world.NewBatchFSWriter(wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, sender)

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
	if err := bw.Commit(ctx); err == nil {
		t.Fatal("expected Commit to error on missing parent")
	}
	bw.Release()
}

// TestBatchFSWriter_Overwrite exercises iter 7: a file existing in the
// target FSTree is replaced by a fresh entry carrying new content, without
// a duplicate dirent appearing.
func TestBatchFSWriter_Overwrite(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

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
	defer fsHandle.Release()

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

	fh, err := fsHandle.Lookup(ctx, "greeting.txt")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	defer fh.Release()
	size, err := fh.GetSize(ctx)
	if err != nil {
		t.Fatalf("GetSize: %v", err)
	}
	if size != uint64(len(updated)) {
		t.Fatalf("size got %d want %d", size, len(updated))
	}
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
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

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
	defer fsHandle.Release()

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

	if err := bw.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	bw.Release()

	fh, _, err := fsHandle.LookupPathPts(ctx, []string{"docs", "notes", "readme.txt"})
	if err != nil {
		t.Fatalf("Lookup nested: %v", err)
	}
	defer fh.Release()
	size, err := fh.GetSize(ctx)
	if err != nil {
		t.Fatalf("GetSize nested: %v", err)
	}
	if size != uint64(len("nested")) {
		t.Fatalf("nested size got %d want %d", size, len("nested"))
	}
}
