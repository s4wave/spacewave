package unixfs_sync

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
)

// TestSyncCarriesModTime checks a synced file takes the source
// modification time, so a later sync matches it by size and mtime, and that a
// source edit with a newer mtime is written again.
func TestSyncCarriesModTime(t *testing.T) {
	// Open a source filesystem whose file has a fixed modification time.
	ctx := t.Context()
	stamp := time.Unix(1000, 0)
	files := fstest.MapFS{
		"note.txt": {Data: []byte("before"), Mode: 0o644, ModTime: stamp},
	}
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

	// Sync to disk and require the file to carry the source mtime.
	directory := t.TempDir()
	filename := filepath.Join(directory, "note.txt")
	checkFile := func(wantData string, wantTime time.Time) {
		t.Helper()
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filename)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != wantData || !info.ModTime().Equal(wantTime) {
			t.Fatalf("file = %q at %v, want %q at %v", data, info.ModTime(), wantData, wantTime)
		}
	}
	if err := Sync(ctx, directory, handle, DeleteMode_DeleteMode_DURING, nil); err != nil {
		t.Fatal(err)
	}
	checkFile("before", stamp)

	// A same-size edit with a newer mtime is written, carrying the new mtime.
	later := stamp.Add(time.Hour)
	files["note.txt"] = &fstest.MapFile{Data: []byte("beyond"), Mode: 0o644, ModTime: later}
	if err := Sync(ctx, directory, handle, DeleteMode_DeleteMode_DURING, nil); err != nil {
		t.Fatal(err)
	}
	checkFile("beyond", later)
}
