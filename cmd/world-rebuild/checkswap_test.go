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

// TestRawCopyKeepsNull checks the raw copy keeps a NULL value NULL, as the
// kvtx SQLite store writes a nil value (copy-done/<target>, for one), and
// count reports the NULL values it saw.
func TestRawCopyKeepsNull(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "raw", 20000
	if _, err := buildTestStore(o.world, o.packs, 200, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", o.world)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{testObjPrefix + "copy-done/remote-1", "h/r/null"} {
		if _, err := db.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte(k), []byte(nil)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte("h/r/empty"), []byte{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	var nulls int64
	if err := db.QueryRow("SELECT count(*) FROM kv WHERE value IS NULL").Scan(&nulls); err != nil {
		t.Fatal(err)
	}
	db.Close()
	res, err := runCount(o)
	if err != nil {
		t.Fatal(err)
	}
	if res.nullValues != nulls || nulls < 2 {
		t.Errorf("count saw %d NULL values, want %d", res.nullValues, nulls)
	}
	if err := runCopy(o); err != nil {
		t.Fatal(err)
	}
	dst := mustOpen(t, o.out)
	for k, want := range map[string]string{testObjPrefix + "copy-done/remote-1": "null", "h/r/null": "null", "h/r/empty": "blob"} {
		var ty string
		if err := dst.QueryRow("SELECT typeof(value) FROM kv WHERE key = ?", []byte(k)).Scan(&ty); err != nil {
			t.Fatal(k, err)
		}
		if ty != want {
			t.Errorf("%s: copied as %s, want %s", k, ty, want)
		}
	}
}

// TestCopyRefusesExtraTable checks copy refuses a source World with a table
// other than its kv table: copy carries the kv table alone.
func TestCopyRefusesExtraTable(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "raw", 20000
	if _, err := buildTestStore(o.world, o.packs, 200, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", o.world)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE extra (x)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := runCopy(o); err == nil {
		t.Fatal("copy of a World with an extra table succeeded")
	}
}

// TestVerifyTellsNullFromEmpty checks verify, raw and through SQLite,
// rejects a copy where a NULL value became an empty blob, or an empty blob
// NULL: the bytes are equal, the types are not.
func TestVerifyTellsNullFromEmpty(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "raw", 20000
	if _, err := buildTestStore(o.world, o.packs, 200, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", o.world)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte("h/r/empty"), []byte{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := runCopy(o); err != nil {
		t.Fatal(err)
	}

	// The faithful copy verifies both ways.
	for _, read := range []string{"raw", "sqlite"} {
		vo := o
		vo.read = read
		if err := runVerify(vo); err != nil {
			t.Fatalf("verify -read %s of the faithful copy: %v", read, err)
		}
	}

	// Flip one value's type in the copy, keeping its bytes.
	for name, q := range map[string]string{
		"NULL to empty blob": "UPDATE kv SET value = x'' WHERE rowid = (SELECT min(rowid) FROM kv WHERE value IS NULL)",
		"empty blob to NULL": "UPDATE kv SET value = NULL WHERE key = CAST('h/r/empty' AS BLOB)",
	} {
		t.Run(name, func(t *testing.T) {
			vo := o
			vo.out = filepath.Join(t.TempDir(), "flipped.db")
			b, err := os.ReadFile(o.out)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(vo.out, b, 0o644); err != nil {
				t.Fatal(err)
			}
			st, err := os.ReadFile(stampPath(o.out))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stampPath(vo.out), st, 0o644); err != nil {
				t.Fatal(err)
			}
			d, err := sql.Open("sqlite", vo.out)
			if err != nil {
				t.Fatal(err)
			}
			r, err := d.Exec(q)
			if err != nil {
				t.Fatal(err)
			}
			if n, _ := r.RowsAffected(); n != 1 {
				t.Fatalf("flipped %d rows, want 1", n)
			}
			if _, err := d.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
				t.Fatal(err)
			}
			d.Close()
			for _, read := range []string{"raw", "sqlite"} {
				vo.read = read
				if err := runVerify(vo); err == nil {
					t.Errorf("verify -read %s passed a copy with a value's type flipped", read)
				}
			}
		})
	}
}
