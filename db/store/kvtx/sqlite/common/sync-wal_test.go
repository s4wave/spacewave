//go:build linux || darwin

package common_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// TestLocalWALSyncsDirOncePerWAL checks that LocalWAL owes a directory flush
// to each WAL file created at the path, never skipping one for a WAL that
// replaced the one it flushed, and reuses the flushed WAL otherwise.
func TestLocalWALSyncsDirOncePerWAL(t *testing.T) {
	// A database without a WAL has nothing to flush.
	dbPath := filepath.Join(t.TempDir(), "db.sqlite")
	walPath := dbPath + "-wal"
	if err := os.WriteFile(dbPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var w common.LocalWAL
	defer w.Close()
	if err := w.Sync(dbPath); err != nil || w.SyncedWAL() != nil {
		t.Fatalf("sync without a WAL: err %v, kept %v", err, w.SyncedWAL())
	}

	// The first WAL is flushed with its entry and kept.
	if err := os.WriteFile(walPath, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(dbPath); err != nil {
		t.Fatal(err)
	}
	first := w.SyncedWAL()
	if first == nil {
		t.Fatal("first WAL not kept after its directory flush")
	}

	// Another commit to the same WAL reuses it without a directory flush.
	if err := w.Sync(dbPath); err != nil {
		t.Fatal(err)
	}
	if w.SyncedWAL() != first {
		t.Fatal("unchanged WAL reopened")
	}

	// A WAL deleted and recreated at the path is a new entry, flushed again.
	if err := os.Remove(walPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walPath, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(dbPath); err != nil {
		t.Fatal(err)
	}
	second := w.SyncedWAL()
	info, err := os.Stat(walPath)
	if err != nil {
		t.Fatal(err)
	}
	held, err := second.Stat()
	if second == first || err != nil || !os.SameFile(held, info) {
		t.Fatal("recreated WAL not flushed with its directory entry")
	}

	// A deleted WAL leaves nothing kept.
	if err := os.Remove(walPath); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(dbPath); err != nil || w.SyncedWAL() != nil {
		t.Fatalf("sync after delete: err %v, kept %v", err, w.SyncedWAL())
	}
}
