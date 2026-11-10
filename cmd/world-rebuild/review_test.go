package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestOtherIDsAreNotOwn checks the drop rule takes facts only from the
// production write-back store id, p/local/hyperline-backend/blk/
// hyperline-world/writeback: a marker of the same block store name under
// another provider or account, or of the bare writeback id, is another
// store's, and copy and count refuse it.
func TestOtherIDsAreNotOwn(t *testing.T) {
	for _, other := range []string{
		"h/objs/writeback/",
		"h/objs/p/other/acct9/blk/hyperline-world/writeback/",
		"h/objs/p/local/acct1/blk/hyperline-world/writeback/",
	} {
		t.Run(other, func(t *testing.T) {
			o := sharedRefWorld(t, func(_, b58 string) []string { return []string{other + "marked/" + b58} })
			if err := runCopy(o); err == nil {
				t.Error("copy succeeded with a marker of " + other)
			}
			if _, err := runCount(o); err == nil {
				t.Error("count succeeded with a marker of " + other)
			}
		})
	}
}

// TestSQLiteCopyKeepsEmptyBlob checks copy -read sqlite keeps an empty blob
// an empty blob, not NULL, and verify -read sqlite passes the copy.
func TestSQLiteCopyKeepsEmptyBlob(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "sqlite", 20000
	if _, err := buildTestStore(o.world, o.packs, 200, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", o.world)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO kv (key, value) VALUES (CAST('h/r/empty' AS BLOB), X'')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := runCopy(o); err != nil {
		t.Fatal(err)
	}
	var ty string
	if err := mustOpen(t, o.out).QueryRow("SELECT typeof(value) FROM kv WHERE key = CAST('h/r/empty' AS BLOB)").Scan(&ty); err != nil {
		t.Fatal(err)
	}
	if ty != "blob" {
		t.Errorf("empty blob copied as %s", ty)
	}
	for _, read := range []string{"sqlite", "raw"} {
		vo := o
		vo.read = read
		if err := runVerify(vo); err != nil {
			t.Errorf("verify -read %s: %v", read, err)
		}
	}
}

// TestStampSeesWALReset checks the source stamp catches a WAL that reset and
// refilled to the same size: its checkpoint sequence and salts (header
// bytes 12-23) or its mtime moved.
func TestStampSeesWALReset(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID = defaultStoreID
	if _, err := buildTestStore(o.world, o.packs, 50, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	wal := o.world + "-wal"
	at := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeWAL := func(salt byte, mtime time.Time) {
		t.Helper()
		b := make([]byte, 4096)
		for i := 12; i < 24; i++ {
			b[i] = salt
		}
		if err := os.WriteFile(wal, b, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(wal, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	for name, c := range map[string]struct {
		salt  byte
		mtime time.Time
		moved bool
	}{
		"unchanged":   {1, at, false},
		"salts moved": {2, at, true},
		"mtime moved": {1, at.Add(time.Minute), true},
		"both moved":  {2, at.Add(time.Minute), true},
	} {
		t.Run(name, func(t *testing.T) {
			writeWAL(1, at)
			if err := stampSources(o); err != nil {
				t.Fatal(err)
			}
			writeWAL(c.salt, c.mtime)
			err := checkStamps(o)
			if c.moved && err == nil {
				t.Error("stamp check passed a WAL that moved at the same size")
			}
			if !c.moved && err != nil {
				t.Error("stamp check of an unmoved WAL:", err)
			}
		})
	}
}

// TestBadSubKeysRefused checks keys of the rebuilt store the store's own
// parsers refuse, or that are none it writes, are refused, not read as a
// fact; and that reads/ keys its parser accepts, which checks only the
// generation, are taken.
func TestBadSubKeysRefused(t *testing.T) {
	b58, _ := testB58("bad")
	hex16 := "0000000000000001"
	var o opts
	o.storeID = defaultStoreID
	for _, key := range []string{
		testObjPrefix + "reads/zzzzzzzzzzzzzzzz/" + hex16,
		testObjPrefix + "evictable/NOTHEXNOTHEXNOTH/" + b58,
		testObjPrefix + "evictable/01/" + b58,
		testObjPrefix + "status/writeback/reads/" + hex16 + "/" + hex16,
		testObjPrefix + "status/writeback/epoch",
		testObjPrefix + "status/writeback/marked/" + b58,
	} {
		f := newFacts(o)
		if f.addKey([]byte(key)); f.badKeys != 1 {
			t.Errorf("%s: %d refused, want 1", key, f.badKeys)
		}
	}
	for _, key := range []string{
		testObjPrefix + "reads/" + hex16 + "/zzzzzzzzzzzzzzzz",
		testObjPrefix + "reads/" + hex16 + "0/" + hex16,
	} {
		f := newFacts(o)
		if f.addKey([]byte(key)); f.badKeys != 0 || len(f.valueKeys) != 1 {
			t.Errorf("%s: %d refused, %d values, want the chunk taken as LoadReads takes it", key, f.badKeys, len(f.valueKeys))
		}
	}
}

// TestBadReadsKeyRefused checks copy and count refuse, without a panic, a
// World with a reads/ key whose generation is not hex.
func TestBadReadsKeyRefused(t *testing.T) {
	o := sharedRefWorld(t, func(_, _ string) []string {
		return []string{testObjPrefix + "reads/zzzzzzzzzzzzzzzz/0000000000000001"}
	})
	if err := runCopy(o); err == nil {
		t.Error("copy succeeded with a reads/ key that is not hex")
	}
	if _, err := runCount(o); err == nil {
		t.Error("count succeeded with a reads/ key that is not hex")
	}
}

// TestCountRefusedWritesNoNeeded checks count leaves no -needed-out file when
// it refuses, and refuses a World with schema copy does not carry.
func TestCountRefusedWritesNoNeeded(t *testing.T) {
	o := sharedRefWorld(t, func(other, b58 string) []string { return []string{other + "marked/" + b58} })
	o.neededOut = filepath.Join(t.TempDir(), "needed.txt")
	if _, err := runCount(o); err == nil {
		t.Fatal("count succeeded with another store's marker")
	}
	if _, err := os.Stat(o.neededOut); !os.IsNotExist(err) {
		t.Errorf("refused count left %s: %v", o.neededOut, err)
	}

	o = sharedRefWorld(t, func(_, _ string) []string { return nil })
	db := mustOpen(t, o.world)
	if _, err := db.Exec("CREATE TABLE extra (x)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	o.neededOut = filepath.Join(t.TempDir(), "needed.txt")
	if _, err := runCount(o); err == nil {
		t.Error("count succeeded on a World with an extra table")
	}
	if _, err := os.Stat(o.neededOut); !os.IsNotExist(err) {
		t.Errorf("refused count left %s: %v", o.neededOut, err)
	}
}
