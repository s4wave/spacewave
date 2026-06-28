package unixfs_sync

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
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

func readWorldUnixFSMetricFile(t *testing.T, ctx context.Context, wtb *world_testbed.Testbed, objKey, name string) ([]byte, *file.File, time.Duration) {
	t.Helper()
	obj, found, err := wtb.WorldState.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatalf("world object %q not found", objKey)
	}
	objRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootCursor, err := wtb.WorldState.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootCursor.Release()
	locCursor, err := rootCursor.FollowRef(ctx, objRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer locCursor.Release()
	_, bcs := locCursor.BuildTransaction(nil)
	root, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	child, _, err := root.LookupFollowDirent(name)
	if err != nil {
		t.Fatal(err.Error())
	}
	fh, err := child.BuildFileHandle(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fh.Close()
	readStarted := time.Now()
	out, err := io.ReadAll(fh)
	readLatency := time.Since(readStarted)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootFile, err := block.UnmarshalBlock[*file.File](ctx, fh.GetCursor(), file.NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	return out, rootFile, readLatency
}

func TestSyncToUnixfsBatch_MetricExistingFileRewrite(t *testing.T) {
	ctx, dstRef, wtb, objKey := buildDstBatchTestbed(t)
	ts := time.Unix(1_700_000_000, 0)
	if err := dstRef.Mknod(ctx, true, []string{"metric.txt"}, unixfs.NewFSCursorNodeType_File(), 0o644, ts); err != nil {
		t.Fatal(err.Error())
	}
	dstFile, err := dstRef.Lookup(ctx, "metric.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer dstFile.Release()
	body := bytes.Repeat([]byte("sync-existing-file-rewrite-base-"), 128)
	seed := append([]byte(nil), body...)
	if err := dstFile.WriteAt(ctx, 0, seed, ts); err != nil {
		t.Fatal(err.Error())
	}
	seedWrites := []struct {
		offset int
		data   []byte
	}{
		{96, bytes.Repeat([]byte("a"), 64)},
		{96, bytes.Repeat([]byte("b"), 64)},
		{220, bytes.Repeat([]byte("c"), 48)},
	}
	for _, write := range seedWrites {
		copy(seed[write.offset:], write.data)
		if err := dstFile.WriteAt(ctx, int64(write.offset), write.data, ts); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := dstFile.Truncate(ctx, uint64(len(seed)), ts); err != nil {
		t.Fatal(err.Error())
	}

	_, beforeFile, _ := readWorldUnixFSMetricFile(t, ctx, wtb, objKey, "metric.txt")
	beforeRefs := rangeRefStrings(beforeFile.GetRanges())
	if len(beforeRefs) == 0 {
		t.Fatal("metric seed did not produce reachable range refs")
	}

	imported := append([]byte(nil), seed...)
	copy(imported[300:], bytes.Repeat([]byte("d"), 24))
	srcHandle := srcHandleFromFS(t, fstest.MapFS{
		"metric.txt": {Data: imported, Mode: 0o644, ModTime: ts},
	})
	defer srcHandle.Release()

	b := unixfs_world.NewBatchFSWriter(
		wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID(),
	)
	rootWriteStarted := time.Now()
	if err := SyncToUnixfsBatch(ctx, b, srcHandle, nil); err != nil {
		t.Fatalf("SyncToUnixfsBatch: %v", err)
	}
	rootWriteLatency := time.Since(rootWriteStarted)

	out, rootFile, readLatency := readWorldUnixFSMetricFile(t, ctx, wtb, objKey, "metric.txt")
	if !bytes.Equal(out, imported) {
		t.Fatal("unixfs sync existing-file rewrite readback mismatch")
	}
	metadataBytes, err := rootFile.MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	afterRefs := rangeRefStrings(rootFile.GetRanges())
	preservedRefs := countPreservedRefs(beforeRefs, afterRefs)
	occluded := countFullyOccludedRanges(rootFile.GetRanges())
	overlapDepth := maxOverlapDepth(rootFile.GetRanges())
	lookupScan := lookupScanLength(rootFile.GetRanges(), 128)
	if occluded != 0 {
		t.Fatalf("sync rewrite left stale occluded ranges: ranges=%d occluded=%d overlap_depth=%d", len(rootFile.GetRanges()), occluded, overlapDepth)
	}
	if len(rootFile.GetRanges()) != 0 && len(afterRefs) == 0 {
		t.Fatalf("sync rewrite produced non-empty range stack without block refs: ranges=%d", len(rootFile.GetRanges()))
	}
	t.Logf("metric workload=unixfs-sync-existing-file-rewrite file_class=unixfs-sync chunk_class=blob before_range_count=%d range_count=%d before_range_refs=%d after_range_refs=%d fully_occluded_range_count=%d stale_reachable_refs=%d overlap_depth=%d lookup_scan_length=%d imported_bytes=%d preserved_range_refs=%d read_latency_ns=%d serialized_metadata_bytes=%d root_write_latency_ns=%d metadata_rewrite_bytes_per_append=%d metadata_rewrite_bytes_per_publish=%d", len(beforeFile.GetRanges()), len(rootFile.GetRanges()), len(beforeRefs), len(afterRefs), occluded, occluded, overlapDepth, lookupScan, len(imported), preservedRefs, readLatency.Nanoseconds(), len(metadataBytes), rootWriteLatency.Nanoseconds(), len(metadataBytes), len(metadataBytes))
}

func rangeRefStrings(ranges []*file.Range) []string {
	refs := make([]string, 0, len(ranges))
	for _, rng := range ranges {
		if rng.GetRef() != nil {
			refs = append(refs, rng.GetRef().MarshalString())
		}
	}
	return refs
}

func countPreservedRefs(before, after []string) int {
	afterSet := make(map[string]struct{}, len(after))
	for _, ref := range after {
		afterSet[ref] = struct{}{}
	}
	var count int
	for _, ref := range before {
		if _, ok := afterSet[ref]; ok {
			count++
		}
	}
	return count
}

func countFullyOccludedRanges(ranges []*file.Range) int {
	var count int
	for i, rng := range ranges {
		if rangeFullyOccluded(i, ranges, rng) {
			count++
		}
	}
	return count
}

func rangeFullyOccluded(idx int, ranges []*file.Range, rng *file.Range) bool {
	start := rng.GetStart()
	end := start + rng.GetLength()
	for pos := start; pos < end; pos++ {
		covered := false
		for j, other := range ranges {
			if j == idx || other.GetNonce() <= rng.GetNonce() {
				continue
			}
			if other.GetStart() <= pos && pos < other.GetStart()+other.GetLength() {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func maxOverlapDepth(ranges []*file.Range) int {
	var maxDepth int
	for _, rng := range ranges {
		end := rng.GetStart() + rng.GetLength()
		for pos := rng.GetStart(); pos < end; pos++ {
			var depth int
			for _, other := range ranges {
				if other.GetStart() <= pos && pos < other.GetStart()+other.GetLength() {
					depth++
				}
			}
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	}
	return maxDepth
}

func lookupScanLength(ranges []*file.Range, pos uint64) int {
	idxAfter := len(ranges)
	for i, rng := range ranges {
		if rng.GetStart() > pos {
			idxAfter = i
			break
		}
	}
	var scans int
	for i := idxAfter - 1; i >= 0; i-- {
		scans++
		rng := ranges[i]
		start := rng.GetStart()
		end := start + rng.GetLength()
		if start <= pos && pos < end {
			continue
		}
	}
	return scans
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
