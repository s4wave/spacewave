package common

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
