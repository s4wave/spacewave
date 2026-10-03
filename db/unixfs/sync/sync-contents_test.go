package unixfs_sync

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
)

// TestSyncContentsPreservesUnchangedFiles catches edits hidden by equal metadata
// without emitting writes for every unchanged source module, and reports each
// destination file it changes.
func TestSyncContentsPreservesUnchangedFiles(t *testing.T) {
	// Prepare a source filesystem whose files have fixed timestamps.
	ctx := t.Context()
	stamp := time.Unix(1000, 0)
	files := fstest.MapFS{
		"viewer.ts":  {Data: []byte("before"), Mode: 0o644, ModTime: stamp},
		"lib":        {Mode: fs.ModeDir | 0o755, ModTime: stamp},
		"lib/one.ts": {Data: []byte("one"), Mode: 0o644, ModTime: stamp},
	}

	// Open the source filesystem for content-aware traversal.
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

	// Prepare the disk destination and its change-reporting sync helper.
	directory := t.TempDir()
	out := osfs.New(directory)
	filename := filepath.Join(directory, "viewer.ts")
	sync := func(want ...string) {
		// Copy source contents while collecting destination change notifications.
		t.Helper()
		var got []string
		err := SyncToBillyContents(ctx, out, handle, DeleteMode_DeleteMode_DURING, nil, func(name string, kind ChangeKind) {
			got = append(got, fmt.Sprintf("%d %s", kind, name))
		})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the sync reports exactly the expected destination changes.
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Fatalf("changes = %q, want %q", got, want)
		}
	}

	// Seed the destination files and pin their modification timestamp.
	sync(fmt.Sprintf("%d lib/one.ts", ChangeCreate), fmt.Sprintf("%d viewer.ts", ChangeCreate))
	if err := os.Chtimes(filename, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	// A second observation of the same source must leave its watcher idle.
	sync()
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(stamp) {
		t.Fatal("unchanged source was rewritten")
	}

	// Same-length bytes with the same mtime still replace the previous module.
	files["viewer.ts"].Data = []byte("edited")
	sync(fmt.Sprintf("%d viewer.ts", ChangeWrite))
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "edited" {
		t.Fatalf("source edit was skipped: %q", data)
	}

	// Removing a directory reports each file it held.
	delete(files, "lib/one.ts")
	sync(fmt.Sprintf("%d lib/one.ts", ChangeRemove))
}
