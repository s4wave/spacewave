package unixfs_billy_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// createInterleaving publishes a host file between Billy's stat and open.
type createInterleaving struct {
	// Filesystem serves the same folder as the host writer.
	billy.Filesystem
	// folder names the host writer's scratch directory.
	folder string
}

// OpenFile publishes the competing upload before opening the entry for Billy.
func (f createInterleaving) OpenFile(name string, flag int, perm os.FileMode) (billy.File, error) {
	// Place the bytes after the preflight lookup and before the opening syscall.
	if err := os.WriteFile(filepath.Join(f.folder, name), []byte("published"), 0o600); err != nil {
		return nil, err
	}

	// Let the filesystem arbitrate the exclusive open.
	return f.Filesystem.OpenFile(name, flag, perm)
}

// TestExclusiveMknodPreservesPublishedFile covers the host writer interleaving.
func TestExclusiveMknodPreservesPublishedFile(t *testing.T) {
	// Serve the scratch folder through the ordinary Billy cursor.
	folder := t.TempDir()
	cursor := unixfs_billy.NewBillyFSCursor(createInterleaving{osfs.New(folder), folder}, "")
	defer cursor.Release()
	ops, err := cursor.GetCursorOps(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// Require a conflict without truncating the competing publication.
	err = ops.Mknod(t.Context(), true, []string{"file"}, unixfs.NewFSCursorNodeType_File(), 0o600, time.Now())
	if !errors.Is(err, unixfs_errors.ErrExist) {
		t.Fatalf("exclusive Mknod = %v, want ErrExist", err)
	}
	data, err := os.ReadFile(filepath.Join(folder, "file"))
	if err != nil || string(data) != "published" {
		t.Fatalf("published file = %q, error %v", data, err)
	}
}
