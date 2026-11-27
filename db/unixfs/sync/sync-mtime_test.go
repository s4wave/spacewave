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

	// Populate read-only source directories before restoring their final modes.
	files["readonly/inner/file.txt"] = &fstest.MapFile{Data: []byte("read-only tree"), Mode: 0o444}
	readonlyCursor, err := unixfs_iofs.NewFSCursor(files)
	if err != nil {
		t.Fatal(err)
	}
	readonlyHandle, err := unixfs.NewFSHandle(readonlyCursor)
	if err != nil {
		readonlyCursor.Release()
		t.Fatal(err)
	}
	defer readonlyHandle.Release()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, name := range []string{"readonly", "readonly/inner"} {
			if err := os.Chmod(filepath.Join(directory, name), 0o700); err != nil && !os.IsNotExist(err) {
				t.Error(err)
			}
		}
	})
	if err := Sync(ctx, directory, readonlyHandle, DeleteMode_DeleteMode_DURING, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "readonly/inner/file.txt"))
	if err != nil || string(data) != "read-only tree" {
		t.Fatalf("read-only directory contents: %q, %v", data, err)
	}

	// Preserve both nested source modes and the private destination root.
	for name, mode := range map[string]os.FileMode{".": 0o700, "readonly": 0o555, "readonly/inner": 0o555} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("directory %s mode: %v, %v", name, info, err)
		}
	}
}
