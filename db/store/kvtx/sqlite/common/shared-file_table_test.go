package common

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestFileTableLateHold takes a hold on a file after its last release, as a
// connection a closed pool was still opening does, then opens a new Store on
// the same path, and releases the late hold. The late hold must join the new
// Store's entry, and its release must leave that entry and its descriptor in
// place: a release that removed or closed them would drop the fcntl locks of
// the new Store's connections.
func TestFileTableLateHold(t *testing.T) {
	// A Store holds the file and writes it back, a connection of its pool
	// holds it, and both release.
	path := filepath.Join(t.TempDir(), "late.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	table := newFileTable()
	store := table.hold(path)
	store.writeBack(path)
	conn := store.again()
	if err := errors.Join(conn.release(), store.release()); err != nil {
		t.Fatal(err)
	}
	if len(table.open) != 0 || store.file.fd != nil {
		t.Fatal("the last release left the entry or its descriptor")
	}

	// A late connection holds the path, then a new Store holds and writes
	// it back.
	late := store.again()
	next := table.hold(path)
	next.writeBack(path)
	if late.file != next.file {
		t.Fatal("the late hold and the new Store hold different entries")
	}

	// The late hold's release leaves the new Store's entry and descriptor.
	if err := late.release(); err != nil {
		t.Fatal(err)
	}
	live := table.open[next.file.path]
	if live != next.file || live.fd == nil {
		t.Fatal("the late release removed the new Store's entry or descriptor")
	}
	if _, err := live.fd.Stat(); err != nil {
		t.Fatal(err)
	}
	if err := next.release(); err != nil {
		t.Fatal(err)
	}
	if len(table.open) != 0 {
		t.Fatal("the last release left the entry")
	}
}

// TestFailedMarkKeepsFile fails a file's mark through its only hold, as a
// failed flush at open does before any pool holds the file, then releases the
// hold and deletes the file. The failed mark must keep a descriptor of the
// file open, so its inode stays allocated and no new file can take its number
// and match the mark, and a new file at the path must open with a healthy
// mark. A healthy mark's descriptor must close with its file's last hold.
func TestFailedMarkKeepsFile(t *testing.T) {
	if kvtx.FailStopOff {
		t.Skip("fail-stop is off")
	}
	path := filepath.Join(t.TempDir(), "failed.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	table := newFileTable()

	// Fail the file through its only hold, release it, and delete the file.
	h := table.hold(path)
	if err := h.markFile(); err != nil {
		t.Fatal(err)
	}
	failed := h.mark
	_ = h.failStop().Fail(errors.New("injected open flush failure"))
	if err := errors.Join(h.release(), os.Remove(path)); err != nil {
		t.Fatal(err)
	}
	if info, err := failed.fd.Stat(); err != nil || !os.SameFile(info, failed.info) {
		t.Fatalf("failed mark's descriptor after the last release and the delete: %v", err)
	}

	// A new file at the path takes a healthy mark, which closes its
	// descriptor with the last hold.
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	next := table.hold(path)
	if err := next.markFile(); err != nil {
		t.Fatal(err)
	}
	if err := next.failStop().Failed(); err != nil || next.mark == failed {
		t.Fatalf("new file at the failed file's path took the failed mark: %v", err)
	}
	healthy := next.mark
	if err := next.release(); err != nil {
		t.Fatal(err)
	}
	if _, err := healthy.fd.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("healthy mark's descriptor after the last release: %v", err)
	}
	if len(table.marks) != 1 || table.marks[0] != failed {
		t.Fatalf("marks after the releases: %d, want only the failed mark", len(table.marks))
	}
}
