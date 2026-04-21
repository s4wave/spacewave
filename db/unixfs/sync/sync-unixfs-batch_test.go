package unixfs_sync

import (
	"bytes"
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/sirupsen/logrus"
)

// buildDstBatchTestbed spins up a UnixFS-backed destination world and
// returns the destination root handle, the underlying world testbed (for
// constructing BatchFSWriter instances), and the object key.
func buildDstBatchTestbed(t *testing.T) (context.Context, *unixfs.FSHandle, *world_testbed.Testbed, string) {
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	objKey := "dst-fs"
	dstRef, wtb, err := unixfs_world_testbed.BuildTestbed(
		btb, objKey, true,
		world_testbed.WithWorldVerbose(false),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ctx, dstRef, wtb, objKey
}

// srcHandleFromFS wraps an io/fs.FS in an FSHandle via the iofs cursor so
// the batch driver walks it via the same interface a real tar source uses.
func srcHandleFromFS(t *testing.T, srcFs fstest.MapFS) *unixfs.FSHandle {
	t.Helper()
	srcCursor, err := unixfs_iofs.NewFSCursor(srcFs)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcHandle, err := unixfs.NewFSHandle(srcCursor)
	if err != nil {
		srcCursor.Release()
		t.Fatal(err.Error())
	}
	return srcHandle
}

// TestSyncToUnixfsBatch_FlatSeed covers Phase 2 iter 1: a flat directory
// source (only regular files at the root) syncs through the batch writer
// and Commit flushes the result with the source contents intact.
func TestSyncToUnixfsBatch_FlatSeed(t *testing.T) {
	ctx, dstRef, wtb, objKey := buildDstBatchTestbed(t)

	src := fstest.MapFS{
		"a.txt": {Data: []byte("alpha"), Mode: 0o644, ModTime: time.Unix(1_700_000_000, 0)},
		"b.txt": {Data: []byte("beta"), Mode: 0o644, ModTime: time.Unix(1_700_000_100, 0)},
	}
	srcHandle := srcHandleFromFS(t, src)
	defer srcHandle.Release()

	b := unixfs_world.NewBatchFSWriter(
		wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID(),
	)
	if err := SyncToUnixfsBatch(ctx, b, srcHandle, nil); err != nil {
		t.Fatalf("SyncToUnixfsBatch: %v", err)
	}

	// Read through the destination and verify both files roundtrip.
	dstBfs := unixfs_billy.NewBillyFS(ctx, dstRef, "", time.Now())
	for name, want := range map[string]string{"a.txt": "alpha", "b.txt": "beta"} {
		got, err := billy_util.ReadFile(dstBfs, name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !bytes.Equal(got, []byte(want)) {
			t.Errorf("%s content = %q, want %q", name, got, want)
		}
	}
}

// TestSyncToUnixfsBatch_Symlinks covers Phase 2 iter 3: a symlinked entry
// in the source roundtrips through AddSymlink and lands with the same
// absolute-vs-relative semantics the source used.
func TestSyncToUnixfsBatch_Symlinks(t *testing.T) {
	ctx, dstRef, wtb, objKey := buildDstBatchTestbed(t)

	// Build a second UnixFS-backed world as the src, using billy to populate
	// a file, a relative symlink, and a symlink pointing into a subdir.
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)
	srcBtb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	srcRef, _, err := unixfs_world_testbed.BuildTestbed(
		srcBtb, "src-fs", true, world_testbed.WithWorldVerbose(false),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcBfs := unixfs_billy.NewBillyFS(ctx, srcRef, "", time.Now())
	if err := billy_util.WriteFile(srcBfs, "b", []byte("file-b-content"), 0o644); err != nil {
		t.Fatal(err.Error())
	}
	if err := srcBfs.Symlink("./b", "a"); err != nil {
		t.Fatal(err.Error())
	}
	if err := srcBfs.MkdirAll("usr/lib64", 0o755); err != nil {
		t.Fatal(err.Error())
	}
	if err := srcBfs.Symlink("usr/lib64", "lib64"); err != nil {
		t.Fatal(err.Error())
	}

	b := unixfs_world.NewBatchFSWriter(
		wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID(),
	)
	if err := SyncToUnixfsBatch(ctx, b, srcRef, nil); err != nil {
		t.Fatalf("SyncToUnixfsBatch: %v", err)
	}

	dstBfs := unixfs_billy.NewBillyFS(ctx, dstRef, "", time.Now())
	for name, want := range map[string]string{"a": "b", "lib64": "usr/lib64"} {
		got, err := dstBfs.Readlink(name)
		if err != nil {
			t.Fatalf("Readlink %s: %v", name, err)
		}
		if got != want {
			t.Errorf("Readlink %s = %q, want %q", name, got, want)
		}
	}
	data, err := billy_util.ReadFile(dstBfs, "b")
	if err != nil {
		t.Fatalf("ReadFile b: %v", err)
	}
	if !bytes.Equal(data, []byte("file-b-content")) {
		t.Errorf("b content mismatch: %q", data)
	}
}

// TestSyncToUnixfsBatch_RootfsShape covers Phase 2 iter 4: a realistic
// rootfs-shaped input (multiple sibling dirs at each level, files mixed in
// with subdirs, varying perms, absolute and relative symlinks) roundtrips
// without tripping the Phase 1 missing-parent guard. Exercises the
// depth-first pre-order walk contract on a non-trivial shape.
func TestSyncToUnixfsBatch_RootfsShape(t *testing.T) {
	ctx, dstRef, wtb, objKey := buildDstBatchTestbed(t)

	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)
	srcBtb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	srcRef, _, err := unixfs_world_testbed.BuildTestbed(
		srcBtb, "src-fs", true, world_testbed.WithWorldVerbose(false),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcBfs := unixfs_billy.NewBillyFS(ctx, srcRef, "", time.Now())

	files := map[string]struct {
		data []byte
		mode fs.FileMode
	}{
		"etc/passwd":            {[]byte("root:x:0:0:::\n"), 0o644},
		"etc/shadow":            {[]byte("root:*::\n"), 0o600},
		"etc/ssh/sshd_config":   {[]byte("Port 22\n"), 0o644},
		"bin/sh":                {[]byte("#!sh\n"), 0o755},
		"usr/bin/env":           {[]byte("env\n"), 0o755},
		"usr/lib/gcc/README":    {[]byte("gcc\n"), 0o644},
		"var/log/syslog":        {[]byte("started\n"), 0o640},
		"var/lib/dpkg/status":   {[]byte("pkg\n"), 0o644},
		"home/user/.bashrc":     {[]byte("PS1=$\n"), 0o644},
		"home/user/docs/readme": {[]byte("hi\n"), 0o644},
	}
	for name, f := range files {
		if err := billy_util.WriteFile(srcBfs, name, f.data, f.mode); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := srcBfs.Symlink("bin/sh", "sh"); err != nil {
		t.Fatal(err.Error())
	}
	if err := srcBfs.Symlink("../usr/bin/env", "bin/env"); err != nil {
		t.Fatal(err.Error())
	}

	b := unixfs_world.NewBatchFSWriter(
		wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID(),
	)
	if err := SyncToUnixfsBatch(ctx, b, srcRef, nil); err != nil {
		t.Fatalf("SyncToUnixfsBatch: %v", err)
	}

	dstBfs := unixfs_billy.NewBillyFS(ctx, dstRef, "", time.Now())
	for name, f := range files {
		got, err := billy_util.ReadFile(dstBfs, name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !bytes.Equal(got, f.data) {
			t.Errorf("%s content = %q, want %q", name, got, f.data)
		}
	}
	for name, want := range map[string]string{"sh": "bin/sh", "bin/env": "../usr/bin/env"} {
		got, err := dstBfs.Readlink(name)
		if err != nil {
			t.Fatalf("Readlink %s: %v", name, err)
		}
		if got != want {
			t.Errorf("Readlink %s = %q, want %q", name, got, want)
		}
	}
}

// TestSyncToUnixfsBatch_NestedDirs covers Phase 2 iter 2: subdirectories
// encountered in the walk are declared via AddDir before any child entries
// are written, so the BatchFSWriter missing-parent guard stays quiet.
func TestSyncToUnixfsBatch_NestedDirs(t *testing.T) {
	ctx, dstRef, wtb, objKey := buildDstBatchTestbed(t)

	src := fstest.MapFS{
		"top.txt":             {Data: []byte("top"), Mode: 0o644, ModTime: time.Unix(1_700_000_000, 0)},
		"dir/inner.txt":       {Data: []byte("inner"), Mode: 0o600, ModTime: time.Unix(1_700_000_100, 0)},
		"dir/sub/deep.txt":    {Data: []byte("deep"), Mode: 0o644, ModTime: time.Unix(1_700_000_200, 0)},
		"dir/sub/sibling.txt": {Data: []byte("sibling"), Mode: 0o644, ModTime: time.Unix(1_700_000_300, 0)},
	}
	srcHandle := srcHandleFromFS(t, src)
	defer srcHandle.Release()

	b := unixfs_world.NewBatchFSWriter(
		wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID(),
	)
	if err := SyncToUnixfsBatch(ctx, b, srcHandle, nil); err != nil {
		t.Fatalf("SyncToUnixfsBatch: %v", err)
	}

	dstBfs := unixfs_billy.NewBillyFS(ctx, dstRef, "", time.Now())
	expected := map[string]string{
		"top.txt":             "top",
		"dir/inner.txt":       "inner",
		"dir/sub/deep.txt":    "deep",
		"dir/sub/sibling.txt": "sibling",
	}
	for name, want := range expected {
		got, err := billy_util.ReadFile(dstBfs, name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !bytes.Equal(got, []byte(want)) {
			t.Errorf("%s content = %q, want %q", name, got, want)
		}
	}
}
