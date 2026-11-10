package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCheckSwap checks copy stamps its sources, and checkswap and verify
// refuse once the source World moves after the copy.
func TestCheckSwap(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "raw", 20000
	if _, err := buildTestStore(o.world, o.packs, 200, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	if err := runCopy(o); err != nil {
		t.Fatal(err)
	}
	if err := runCheckSwap(o); err != nil {
		t.Fatal("checkswap of an unmoved source:", err)
	}

	// Touching the mtime alone is a move.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(o.world, later, later); err != nil {
		t.Fatal(err)
	}
	if err := runCheckSwap(o); err == nil {
		t.Fatal("checkswap passed a source with a new mtime")
	}

	// So is a committed write.
	db, err := sql.Open("sqlite", o.world)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte("h/r/late"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := runCheckSwap(o); err == nil {
		t.Fatal("checkswap passed a source written after the copy")
	}
	if err := runVerify(o); err == nil {
		t.Fatal("verify passed a source written after the copy")
	}
}
