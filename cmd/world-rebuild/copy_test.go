package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

// rowsHash hashes every row of a copy in key order, and the schema rows.
func rowsHash(t *testing.T, path string) (rows int64, sum string, schema string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r, err := db.Query("SELECT key, value FROM kv ORDER BY key")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	h := sha256.New()
	for r.Next() {
		var k, v []byte
		if err := r.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(h, "%d:%x %d:%x\n", len(k), k, len(v), v)
		rows++
	}
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	return rows, fmt.Sprintf("%x", h.Sum(nil)), masterRows(t, db)
}

// masterRows lists sqlite_master without the root pages.
func masterRows(t *testing.T, db *sql.DB) string {
	t.Helper()
	r, err := db.Query("SELECT type, name, tbl_name, sql FROM sqlite_master ORDER BY rowid")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s := ""
	for r.Next() {
		var ty, name, tbl string
		var q sql.NullString
		if err := r.Scan(&ty, &name, &tbl, &q); err != nil {
			t.Fatal(err)
		}
		s += fmt.Sprintf("%s|%s|%s|%v|%q\n", ty, name, tbl, q.Valid, q.String)
	}
	return s
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
	srcSchema := masterRows(t, src)
	src.Close()

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
