package unixfs_sync

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
)

// TestSyncContentsPreservesUnchangedFiles catches edits hidden by equal metadata
// without emitting writes for every unchanged source module.
func TestSyncContentsPreservesUnchangedFiles(t *testing.T) {
	ctx := t.Context()
	stamp := time.Unix(1000, 0)
	files := fstest.MapFS{"viewer.ts": {Data: []byte("before"), Mode: 0o644, ModTime: stamp}}
	cursor, err := unixfs_iofs.NewFSCursor(files)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		t.Fatal(err)
	}
	defer handle.Release()
	directory := t.TempDir()
	out := osfs.New(directory)
	filename := filepath.Join(directory, "viewer.ts")
	if err := SyncToBillyContents(ctx, out, handle, DeleteMode_DeleteMode_DURING, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filename, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	// A second observation of the same source must leave its watcher idle.
	if err := SyncToBillyContents(ctx, out, handle, DeleteMode_DeleteMode_DURING, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(stamp) {
		t.Fatal("unchanged source was rewritten")
	}

	// Same-length bytes with the same mtime still replace the previous module.
	files["viewer.ts"].Data = []byte("edited")
	if err := SyncToBillyContents(ctx, out, handle, DeleteMode_DeleteMode_DURING, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "edited" {
		t.Fatalf("source edit was skipped: %q", data)
	}
}
