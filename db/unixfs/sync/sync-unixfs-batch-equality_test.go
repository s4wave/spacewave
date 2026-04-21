package unixfs_sync

import (
	"archive/tar"
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_tar "github.com/s4wave/spacewave/db/unixfs/tar"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// rootfsFixtureTime pins the timestamp in every tar entry so both import
// paths see identical mtime inputs. A drifting clock would create
// superficial rootRef divergence unrelated to the batch-vs-per-op contract.
var rootfsFixtureTime = time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

// buildRootfsFixture writes a compact rootfs-shaped tar archive exercising
// regular files, a nested directory, and a symlink. The layout matches the
// OQ-5 contract: every file and symlink sits under a parent that is either
// the root (assumed to exist) or explicitly declared via a TypeDir entry
// earlier in the stream.
func buildRootfsFixture() []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeHeader := func(hdr *tar.Header) {
		hdr.ModTime = rootfsFixtureTime
		_ = tw.WriteHeader(hdr)
	}
	writeFile := func(name, content string, mode int64) {
		writeHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Mode:     mode,
			Typeflag: tar.TypeReg,
		})
		_, _ = tw.Write([]byte(content))
	}
	writeDir := func(name string, mode int64) {
		writeHeader(&tar.Header{
			Name:     name,
			Mode:     mode,
			Typeflag: tar.TypeDir,
		})
	}
	writeSymlink := func(name, target string) {
		writeHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeSymlink,
			Linkname: target,
		})
	}

	writeDir("etc/", 0o755)
	writeFile("etc/passwd", "root:x:0:0:::\n", 0o644)
	writeFile("etc/shadow", "root:*::\n", 0o600)
	writeDir("bin/", 0o755)
	writeFile("bin/sh", "#!sh\n", 0o755)
	writeFile("README", "rootfs fixture\n", 0o644)
	writeSymlink("link-readme", "README")

	_ = tw.Close()
	return buf.Bytes()
}

// runImportCapturingRootRef builds a fresh UnixFS testbed, runs one of the
// two import paths against the tar fixture, and returns the resulting
// world-object root ref for comparison.
func runImportCapturingRootRef(t *testing.T, tarBytes []byte, useBatch bool) *bucket.ObjectRef {
	t.Helper()
	return runImportsCapturingRootRef(t, [][]byte{tarBytes}, useBatch)
}

// runImportsCapturingRootRef builds a fresh UnixFS testbed and runs one of
// the two import paths over each tar payload in sequence against the same
// world object, returning the resulting root ref after the final import.
// Each batch run allocates a fresh BatchFSWriter so the overwrite pass goes
// through the merge-into-existing-FSTree path rather than reusing in-memory
// accumulator state from the first pass.
func runImportsCapturingRootRef(t *testing.T, tarPayloads [][]byte, useBatch bool) *bucket.ObjectRef {
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	le := logrus.NewEntry(log)

	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	objKey := "rootfs"
	dstRef, wtb, err := unixfs_world_testbed.BuildTestbed(
		btb, objKey, true,
		world_testbed.WithWorldVerbose(false),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	for i, tarBytes := range tarPayloads {
		ra := bytes.NewReader(tarBytes)
		tarCursor, err := unixfs_tar.NewTarFSCursor(ra, int64(ra.Len()))
		if err != nil {
			t.Fatalf("NewTarFSCursor pass %d: %v", i, err)
		}
		srcHandle, err := unixfs.NewFSHandle(tarCursor)
		if err != nil {
			tarCursor.Release()
			t.Fatalf("NewFSHandle pass %d: %v", i, err)
		}

		if useBatch {
			b := unixfs_world.NewBatchFSWriter(
				wtb.WorldState, objKey, unixfs_world.FSType_FSType_FS_NODE, wtb.Volume.GetPeerID(),
			)
			if err := SyncToUnixfsBatch(ctx, b, srcHandle, nil); err != nil {
				srcHandle.Release()
				tarCursor.Release()
				t.Fatalf("SyncToUnixfsBatch pass %d: %v", i, err)
			}
		} else {
			// Pin the billy op timestamp so per-op writes stamp every entry
			// with the same mtime the batch path lifts off the source.
			// Without this, billy defaults to time.Now() per write and root
			// refs diverge purely on dirent timestamps.
			bfs := unixfs_billy.NewBillyFS(ctx, dstRef, "", rootfsFixtureTime)
			if err := SyncToBilly(ctx, bfs, srcHandle, DeleteMode_DeleteMode_NONE, nil); err != nil {
				srcHandle.Release()
				tarCursor.Release()
				t.Fatalf("SyncToBilly pass %d: %v", i, err)
			}
		}
		srcHandle.Release()
		tarCursor.Release()
	}

	obj, err := world.MustGetObject(ctx, wtb.WorldState, objKey)
	if err != nil {
		t.Fatalf("MustGetObject: %v", err)
	}
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatalf("GetRootRef: %v", err)
	}
	if ref == nil {
		t.Fatalf("nil root ref after %s import", labelFor(useBatch))
	}
	return ref.Clone()
}

// labelFor returns a short label for the import mode for diagnostics.
func labelFor(useBatch bool) string {
	if useBatch {
		return "batch"
	}
	return "per-op"
}

// TestRootRefEqualAcrossModes is the Phase 4 IC-1 contract: running the
// per-op import path and the batch import path against the same tar input
// must produce byte-equal world RootRefs.
func TestRootRefEqualAcrossModes(t *testing.T) {
	fixture := buildRootfsFixture()

	perOp := runImportCapturingRootRef(t, fixture, false)
	batch := runImportCapturingRootRef(t, fixture, true)

	if !perOp.EqualsRef(batch) {
		t.Fatalf("rootRef mismatch\n  per-op = %+v\n  batch  = %+v", perOp, batch)
	}
}

// buildRootfsFixtureOverwrite returns a tar payload with the same layout as
// buildRootfsFixture but with etc/passwd's contents replaced. Imported as a
// second pass on top of buildRootfsFixture, this exercises the overwrite
// path on both the per-op (billy Create over an existing file) and batch
// (BatchFSWriter in-place dirent NodeRef replacement) implementations.
func buildRootfsFixtureOverwrite() []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeHeader := func(hdr *tar.Header) {
		hdr.ModTime = rootfsFixtureTime
		_ = tw.WriteHeader(hdr)
	}
	writeFile := func(name, content string, mode int64) {
		writeHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Mode:     mode,
			Typeflag: tar.TypeReg,
		})
		_, _ = tw.Write([]byte(content))
	}
	writeDir := func(name string, mode int64) {
		writeHeader(&tar.Header{
			Name:     name,
			Mode:     mode,
			Typeflag: tar.TypeDir,
		})
	}

	writeDir("etc/", 0o755)
	writeFile("etc/passwd", "root:x:0:0:overwritten:\n", 0o644)

	_ = tw.Close()
	return buf.Bytes()
}

// TestRootRefEqualAfterOverwrite covers Phase 4 iter 4: the per-op and batch
// paths must agree on the final rootRef after a file is written, then
// overwritten by a second import pass. Exercises Phase 1 iter 7's
// in-place-dirent-replacement branch against the billy Create-over-existing
// path.
func TestRootRefEqualAfterOverwrite(t *testing.T) {
	first := buildRootfsFixture()
	second := buildRootfsFixtureOverwrite()
	payloads := [][]byte{first, second}

	perOp := runImportsCapturingRootRef(t, payloads, false)
	batch := runImportsCapturingRootRef(t, payloads, true)

	if !perOp.EqualsRef(batch) {
		t.Fatalf("rootRef mismatch after overwrite\n  per-op = %+v\n  batch  = %+v", perOp, batch)
	}
}

// buildDeepRootfsFixture returns a tar payload exercising 4 levels of
// nested directories (usr/lib/gcc/include/) so that Phase 1 iter 6's
// multi-directory post-order Commit walk is covered by an equality
// assertion. The root entry count is kept small so the test stays fast.
func buildDeepRootfsFixture() []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeHeader := func(hdr *tar.Header) {
		hdr.ModTime = rootfsFixtureTime
		_ = tw.WriteHeader(hdr)
	}
	writeFile := func(name, content string, mode int64) {
		writeHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Mode:     mode,
			Typeflag: tar.TypeReg,
		})
		_, _ = tw.Write([]byte(content))
	}
	writeDir := func(name string, mode int64) {
		writeHeader(&tar.Header{
			Name:     name,
			Mode:     mode,
			Typeflag: tar.TypeDir,
		})
	}
	writeSymlink := func(name, target string) {
		writeHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeSymlink,
			Linkname: target,
		})
	}

	writeDir("usr/", 0o755)
	writeDir("usr/lib/", 0o755)
	writeDir("usr/lib/gcc/", 0o755)
	writeDir("usr/lib/gcc/include/", 0o755)
	writeFile("usr/lib/gcc/include/stddef.h", "/* stddef */\n", 0o644)
	writeFile("usr/lib/gcc/include/stdarg.h", "/* stdarg */\n", 0o644)
	writeDir("usr/lib/gcc/lib/", 0o755)
	writeFile("usr/lib/gcc/lib/libgcc.a", "gcc archive\n", 0o644)
	writeFile("usr/lib/README", "usr/lib readme\n", 0o644)
	writeSymlink("usr/lib/gcc/latest", "include")

	_ = tw.Close()
	return buf.Bytes()
}

// TestRootRefEqualDeepNesting covers Phase 4 iter 5: the per-op and batch
// paths must agree on rootRef for a tree with 4 levels of nested
// directories, with files and a symlink mixed across several intermediate
// depths. This is the structural stress test for Phase 1 iter 6's
// depth-ordered Commit merge and the dirty-cursor propagation that feeds
// the final btx.Write.
func TestRootRefEqualDeepNesting(t *testing.T) {
	fixture := buildDeepRootfsFixture()

	perOp := runImportCapturingRootRef(t, fixture, false)
	batch := runImportCapturingRootRef(t, fixture, true)

	if !perOp.EqualsRef(batch) {
		t.Fatalf("rootRef mismatch for deep nesting\n  per-op = %+v\n  batch  = %+v", perOp, batch)
	}
}
