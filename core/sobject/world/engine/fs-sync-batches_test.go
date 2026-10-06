package sobject_world_engine_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/core/sobject"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// syncBatchesFileCount is the number of files in the tree of the batch tests.
// With the long names, their operations exceed sobject.MaxInnerDataSize.
const syncBatchesFileCount = 1500

// syncBatchesDirSize is the number of files in each directory of the tree.
const syncBatchesDirSize = 50

// syncBatchesObjectKey is the UnixFS object the batch tests sync into.
const syncBatchesObjectKey = "files"

// syncBatchesPath is the path of file i, long enough to size the operations.
func syncBatchesPath(i int) string {
	return filepath.Join("dir"+strconv.Itoa(i/syncBatchesDirSize), strings.Repeat("n", 200)+strconv.Itoa(i))
}

// writeSyncBatchesTree writes the source tree of syncBatchesFileCount files.
func writeSyncBatchesTree(t *testing.T) string {
	t.Helper()

	// Write each file under a directory of syncBatchesDirSize files.
	src := t.TempDir()
	for i := range syncBatchesFileCount {
		path := filepath.Join(src, syncBatchesPath(i))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content "+strconv.Itoa(i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

// syncDirToFiles mirrors the source directory into the UnixFS object in ws,
// as the fs sync command does.
func syncDirToFiles(ctx context.Context, ws world.WorldState, src string, mode unixfs_sync.DeleteMode) error {
	return syncFSToFiles(ctx, ws, os.DirFS(src), mode)
}

// syncFSToFiles mirrors the source filesystem into the UnixFS object in ws.
func syncFSToFiles(ctx context.Context, ws world.WorldState, src fs.FS, mode unixfs_sync.DeleteMode) error {
	// Open the UnixFS root for writing, then sync the source into it.
	ref := &unixfs_world.UnixfsRef{ObjectKey: syncBatchesObjectKey}
	root, err := unixfs_world.BuildFSFromUnixfsRef(ctx, nil, ws, "", ref, false, true, time.Now())
	if err != nil {
		return err
	}
	defer root.Release()
	return unixfs_sync.SyncFromFS(ctx, root, src, mode, nil)
}

// initSyncBatchesObject creates the UnixFS object holding the file "stale".
func initSyncBatchesObject(ctx context.Context, t *testing.T, sw *spaceWorld) {
	// Write the destination-only file.
	t.Helper()
	staleSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(staleSrc, "stale"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Initialize the object and sync the file in one commit.
	tx, err := sw.eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	fsType := unixfs_world.FSType_FSType_FS_NODE
	if _, _, err := unixfs_world.FsInit(ctx, tx, sw.sender, syncBatchesObjectKey, fsType, nil, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := syncDirToFiles(ctx, tx, staleSrc, unixfs_sync.DeleteMode_DeleteMode_NONE); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// checkSyncBatchesTree checks the Space holds every file of the source tree
// and holds "stale" only if wantStale.
func checkSyncBatchesTree(ctx context.Context, t *testing.T, sw *spaceWorld, wantStale bool) {
	// Open the UnixFS object in a read transaction.
	t.Helper()
	tx, err := sw.eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	ref := &unixfs_world.UnixfsRef{ObjectKey: syncBatchesObjectKey}
	root, err := unixfs_world.BuildFSFromUnixfsRef(ctx, nil, tx, "", ref, false, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Release()

	// Read every file back.
	bfs := unixfs_billy.NewBillyFS(ctx, root, "", time.Time{})
	for i := range syncBatchesFileCount {
		got, err := billy_util.ReadFile(bfs, syncBatchesPath(i))
		if err != nil {
			t.Fatal(err)
		}
		if want := "content " + strconv.Itoa(i); string(got) != want {
			t.Fatalf("file %d = %q, want %q", i, got, want)
		}
	}

	// Check the destination-only file followed the delete mode.
	_, err = bfs.Stat("stale")
	if wantStale != (err == nil) {
		t.Fatalf("stat stale = %v, want present %v", err, wantStale)
	}
}

// TestSyncBatchesLargeTree syncs a tree whose single-commit operation exceeds
// the SharedObject operation size limit and checks the Space holds every file.
func TestSyncBatchesLargeTree(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      unixfs_sync.DeleteMode
		wantStale bool
	}{
		{"delete after", unixfs_sync.DeleteMode_DeleteMode_AFTER, false},
		{"keep extra", unixfs_sync.DeleteMode_DeleteMode_NONE, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build a Space holding the source tree and a destination-only file.
			ctx := t.Context()
			sw := newSpaceWorld(ctx, t)
			src := writeSyncBatchesTree(t)
			initSyncBatchesObject(ctx, t, sw)

			// Check one transaction cannot hold the tree.
			big, err := sw.eng.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer big.Discard()
			if err := syncDirToFiles(ctx, big, src, tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := big.Commit(ctx); !errors.Is(err, sobject.ErrMaxSizeExceeded) {
				t.Fatalf("single-commit sync = %v, want %v", err, sobject.ErrMaxSizeExceeded)
			}

			// Sync the tree in batches.
			var rounds int
			err = world_block_tx.CommitBatches(ctx, sw.eng, sobject.MaxBatchSize, func(ctx context.Context, ws world.WorldState) error {
				rounds++
				return syncDirToFiles(ctx, ws, src, tc.mode)
			})
			if err != nil {
				t.Fatal(err)
			}
			if rounds < 2 {
				t.Fatalf("synced in %d rounds, want several", rounds)
			}
			checkSyncBatchesTree(ctx, t, sw, tc.wantStale)
		})
	}
}
