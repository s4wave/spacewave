package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rowsHash is copyDigest, failing the test on an error.
func rowsHash(t *testing.T, path string) (int64, string, string) {
	t.Helper()
	rows, sum, schema, err := copyDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	return rows, sum, schema
}

// TestRawCopyMatchesSQLiteCopy copies the selftest store through SQLite and
// raw, pipelined, with each insert option, and checks each copy holds the
// same rows, verifies, and has the source's schema.
func TestRawCopyMatchesSQLiteCopy(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.storeID = defaultStoreID
	o.chunk = 500
	o.full = true
	o.txnRows = 20000
	o.readers = 4
	if _, err := buildTestStore(o.world, o.packs, 3000, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	src, err := sql.Open("sqlite", "file:"+o.world+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	srcSchema, err := masterRows(src)
	src.Close()
	if err != nil {
		t.Fatal(err)
	}

	o.read = "sqlite"
	o.out = filepath.Join(dir, "sqlite.db")
	if err := runCopy(o); err != nil {
		t.Fatal("sqlite copy:", err)
	}
	wantRows, wantSum, schema := rowsHash(t, o.out)
	if schema != srcSchema {
		t.Fatalf("sqlite copy schema:\n%s\nsource:\n%s", schema, srcSchema)
	}

	for _, c := range []struct {
		name string
		mod  func(*opts)
	}{
		{"raw", func(*opts) {}},
		{"raw-sorted", func(o *opts) { o.sortBatch = true }},
		{"raw-big-txn", func(o *opts) { o.txnRows = 1 << 30 }},
		{"raw-defer-index", func(o *opts) { o.deferIndex = true }},
		{"raw-1-reader", func(o *opts) { o.readers = 1 }},
	} {
		ro := o
		ro.read = "raw"
		c.mod(&ro)
		ro.out = filepath.Join(dir, c.name+".db")
		if err := runCopy(ro); err != nil {
			t.Fatalf("%s copy: %v", c.name, err)
		}
		rows, sum, schema := rowsHash(t, ro.out)
		if rows != wantRows || sum != wantSum {
			t.Fatalf("%s copy: %d rows hash %s, sqlite copy %d rows hash %s", c.name, rows, sum, wantRows, wantSum)
		}
		if schema != srcSchema {
			t.Fatalf("%s copy schema:\n%s\nsource:\n%s", c.name, schema, srcSchema)
		}
		for _, read := range []string{"sqlite", "raw"} {
			vo := ro
			vo.read = read
			if err := runVerify(vo); err != nil {
				t.Fatalf("%s copy, %s verify: %v", c.name, read, err)
			}
		}
	}

	// A raw verify catches a mutant as the SQLite verify does.
	for _, m := range []string{"drop-hot", "stale-evict-bytes"} {
		mo := o
		mo.read, mo.mutant = "raw", m
		mo.out = filepath.Join(dir, m+".db")
		if err := runCopy(mo); err != nil {
			t.Fatalf("mutant %s copy: %v", m, err)
		}
		mo.mutant = ""
		if err := runVerify(mo); err == nil {
			t.Fatalf("mutant %s: raw verify passed, want a fault", m)
		}
	}
}

// TestRawCopyRefusesWAL checks a raw copy refuses a source whose WAL holds
// frames, which the raw reader would miss.
func TestRawCopyRefusesWAL(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.storeID, o.chunk, o.readers, o.read = defaultStoreID, 500, 2, "raw"
	o.out = filepath.Join(dir, "new.db")
	if _, err := buildTestStore(o.world, o.packs, 200, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.world+"-wal", make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCopy(o)
	if err == nil || !strings.Contains(err.Error(), "-wal holds") {
		t.Fatalf("raw copy of a source with a WAL: %v, want a refusal", err)
	}
	if _, err := os.Stat(o.out); err == nil {
		t.Fatalf("refused copy wrote %s", o.out)
	}
}
