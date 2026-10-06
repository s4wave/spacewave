package sobject_world_engine_test

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// readCountFS counts the content reads of each file opened through it.
type readCountFS struct {
	// root is the filesystem whose files are counted.
	root fs.FS
	// mtx guards reads.
	mtx sync.Mutex
	// reads maps each file name to its number of content reads.
	reads map[string]int
}

// reset clears the read counts and returns the counts before the reset.
func (c *readCountFS) reset() map[string]int {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	reads := c.reads
	c.reads = make(map[string]int)
	return reads
}

// Open opens the named file with its content reads counted.
// Directories are returned as opened.
func (c *readCountFS) Open(name string) (fs.File, error) {
	f, err := c.root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if info.IsDir() {
		return f, nil
	}
	at, ok := f.(io.ReaderAt)
	if !ok {
		_ = f.Close()
		return nil, errors.Errorf("file %s does not support ReadAt", name)
	}
	return &readCountFile{File: f, at: at, fs: c, name: name}, nil
}

// readCountFile reports each content read of its file to a readCountFS.
type readCountFile struct {
	fs.File
	// at reads the content of the file.
	at io.ReaderAt
	// fs is the filesystem counting the reads.
	fs *readCountFS
	// name is the name the file was opened with.
	name string
}

// ReadAt reads file content at an offset and counts the read.
func (f *readCountFile) ReadAt(p []byte, off int64) (int, error) {
	f.fs.mtx.Lock()
	f.fs.reads[f.name]++
	f.fs.mtx.Unlock()
	return f.at.ReadAt(p, off)
}

// TestSyncBatchesSkipsUnchangedFiles syncs a tree in batches, then again, and
// checks the second sync matches every file by size and mtime without reading
// its content, and that an edited file is read and updated.
func TestSyncBatchesSkipsUnchangedFiles(t *testing.T) {
	// Build a Space holding the first upload of the tree.
	ctx := t.Context()
	sw := newSpaceWorld(ctx, t)
	dir := writeSyncBatchesTree(t)
	initSyncBatchesObject(ctx, t, sw)
	src := &readCountFS{root: os.DirFS(dir), reads: make(map[string]int)}
	syncTree := func() int {
		t.Helper()
		var rounds int
		err := world_block_tx.CommitBatches(ctx, sw.eng, syncBatchesBudget, func(ctx context.Context, ws world.WorldState) error {
			rounds++
			return syncFSToFiles(ctx, ws, src, unixfs_sync.DeleteMode_DeleteMode_NONE)
		})
		if err != nil {
			t.Fatal(err)
		}
		return rounds
	}
	if rounds := syncTree(); rounds < 2 {
		t.Fatalf("synced in %d rounds, want several", rounds)
	}
	if reads := src.reset(); len(reads) < syncBatchesFileCount {
		t.Fatalf("first upload read %d files, want %d", len(reads), syncBatchesFileCount)
	}

	// A rerun matches every file by size and mtime.
	syncTree()
	if reads := src.reset(); len(reads) != 0 {
		t.Fatalf("rerun read %d files, want none", len(reads))
	}

	// An edited file of the same size is read and updated; no other file is.
	edited := syncBatchesPath(7)
	editedPath := filepath.Join(dir, edited)
	if err := os.WriteFile(editedPath, []byte("changed 7"), 0o644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(time.Hour)
	if err := os.Chtimes(editedPath, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	syncTree()
	if reads := src.reset(); len(reads) != 1 || reads[edited] == 0 {
		t.Fatalf("edit sync read %v, want only %s", reads, edited)
	}

	// Read the edited file back from the Space.
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
	got, err := billy_util.ReadFile(unixfs_billy.NewBillyFS(ctx, root, "", time.Time{}), edited)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed 7" {
		t.Fatalf("edited file = %q, want %q", got, "changed 7")
	}
}
