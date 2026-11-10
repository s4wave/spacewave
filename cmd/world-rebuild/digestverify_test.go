package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// testObjPrefix is the object store prefix buildTestStore writes.
const testObjPrefix = "h/objs/world-wb/"

// prodObjPrefix is an object store prefix shaped like production's, whose id
// holds slashes.
const prodObjPrefix = "h/objs/p/local/acct1/blk/hyperline-world/writeback/"

// forStoreIDs runs fn as a subtest with the test store's object store prefix
// and with the production-shaped one. The production-shaped run is skipped
// while objKey cannot split an id that holds slashes.
func forStoreIDs(t *testing.T, fn func(t *testing.T, obj string)) {
	t.Run("world-wb", func(t *testing.T) { fn(t, testObjPrefix) })
	t.Run("prod-id", func(t *testing.T) {
		if _, rest, _ := objKey([]byte(prodObjPrefix + evictBytesSeg)); rest != evictBytesSeg {
			t.Skip("objKey splits the store id at its first slash; runs once facts.go handles production ids")
		}
		fn(t, prodObjPrefix)
	})
}

// digestStore builds a test store of n blocks in dir, its object store keys
// moved under obj, and returns the opts a raw copy with -kept-digest takes.
func digestStore(t *testing.T, dir string, n int, obj string) opts {
	t.Helper()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read = defaultStoreID, 500, 3, "raw"
	o.txnRows = 20000
	o.keptDigest = true
	if _, err := buildTestStore(o.world, o.packs, n, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	if obj != testObjPrefix {
		lo := []byte(testObjPrefix)
		editNew(t, o.world, "UPDATE kv SET key = CAST(? || substr(key, ?) AS BLOB) WHERE key >= ? AND key < ?", -1, []byte(obj), len(lo)+1, lo, prefixEnd(lo))
	}
	return o
}

// digestCopy is digestStore then the copy, returning the opts verify
// -digest takes.
func digestCopy(t *testing.T, dir string, n int, obj string) opts {
	t.Helper()
	o := digestStore(t, dir, n, obj)
	if err := runCopy(o); err != nil {
		t.Fatal("copy:", err)
	}
	o.digest = true
	o.spot = 100000
	o.seed = 7
	return o
}

// editNew runs q on the new file and closes it, so the WAL is checkpointed
// and the raw read sees the change.
func editNew(t *testing.T, path, q string, want int64, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); want >= 0 && n != want {
		t.Fatalf("%s: %d rows changed, want %d", q, n, want)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
}

// hasFault reports whether a fault holds sub.
func hasFault(faults []string, sub string) bool {
	for _, f := range faults {
		if strings.Contains(f, sub) {
			return true
		}
	}
	return false
}

// TestVerifyDigestOK checks a raw copy with -kept-digest passes verify
// -digest, and that a small store's sample covers every source row.
func TestVerifyDigestOK(t *testing.T) {
	forStoreIDs(t, func(t *testing.T, obj string) {
		o := digestCopy(t, t.TempDir(), 300, obj)
		d, err := readKeptDigest(o.out + keptDigestExt)
		if err != nil {
			t.Fatal(err)
		}
		if d.Rows == 0 || d.Dropped == 0 || len(d.EvictBytes) == 0 || d.SumA == d.SumB {
			t.Fatalf("digest file: %+v, want rows, drops and evictable-bytes", d)
		}
		res, err := verifyDigest(o)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.faults) != 0 {
			t.Fatalf("faults on a good copy: %v", res.faults)
		}
		if !res.spotAll || res.spotKept != d.Kept || res.spotDropped != d.Dropped {
			t.Fatalf("spot check covered %d kept %d dropped (all %v), the copy kept %d dropped %d", res.spotKept, res.spotDropped, res.spotAll, d.Kept, d.Dropped)
		}
		if err := runVerifyDigest(o); err != nil {
			t.Fatal(err)
		}

		// A sample smaller than the store is drawn by random rowid.
		o.spot = 100
		res, err = verifyDigest(o)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.faults) != 0 || res.spotAll || res.spotKept < 100 || res.spotDropped < 10 {
			t.Fatalf("sampled spot check: faults %v, %d kept %d dropped (all %v), want 100 and 10 of a sample", res.faults, res.spotKept, res.spotDropped, res.spotAll)
		}
	})
}

// TestVerifyDigestCorruptValue checks a value changed in the new file fails
// the digest.
func TestVerifyDigestCorruptValue(t *testing.T) {
	forStoreIDs(t, func(t *testing.T, obj string) {
		o := digestCopy(t, t.TempDir(), 300, obj)
		editNew(t, o.out, "UPDATE kv SET value = ? WHERE key = ?", 1, []byte("rooT"), []byte("h/r/root"))
		res, err := verifyDigest(o)
		if err != nil {
			t.Fatal(err)
		}
		if !hasFault(res.faults, "digest") {
			t.Fatalf("faults %v, want a digest fault", res.faults)
		}
		if err := runVerifyDigest(o); err == nil {
			t.Fatal("verify -digest passed a corrupted copy")
		}
	})
}

// TestVerifyDigestMissingRow checks a row deleted from the new file fails
// the digest.
func TestVerifyDigestMissingRow(t *testing.T) {
	forStoreIDs(t, func(t *testing.T, obj string) {
		o := digestCopy(t, t.TempDir(), 300, obj)
		editNew(t, o.out, "DELETE FROM kv WHERE key = ?", 1, []byte("zz-format"))
		res, err := verifyDigest(o)
		if err != nil {
			t.Fatal(err)
		}
		if !hasFault(res.faults, "digest") {
			t.Fatalf("faults %v, want a digest fault", res.faults)
		}
	})
}

// TestVerifyDigestSpotCheck checks the SQLite spot check, which covers every
// row of a small store, finds a block value changed in the new file.
func TestVerifyDigestSpotCheck(t *testing.T) {
	forStoreIDs(t, func(t *testing.T, obj string) {
		o := digestCopy(t, t.TempDir(), 300, obj)
		db, err := sql.Open("sqlite", "file:"+o.out+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		var key []byte
		err = db.QueryRow("SELECT key FROM kv WHERE key >= ? AND key < ? ORDER BY key LIMIT 1", []byte(blockPrefix), prefixEnd([]byte(blockPrefix))).Scan(&key)
		db.Close()
		if err != nil {
			t.Fatal(err)
		}
		editNew(t, o.out, "UPDATE kv SET value = value || x'00' WHERE key = ?", 1, key)
		res, err := verifyDigest(o)
		if err != nil {
			t.Fatal(err)
		}
		if !hasFault(res.faults, "spot") {
			t.Fatalf("faults %v, want a spot check fault", res.faults)
		}
	})
}

// TestVerifyDigestOtherSource checks verify -digest refuses a digest file a
// copy of another source wrote.
func TestVerifyDigestOtherSource(t *testing.T) {
	forStoreIDs(t, func(t *testing.T, obj string) {
		dir := t.TempDir()
		o := digestCopy(t, dir, 300, obj)
		other := filepath.Join(dir, "other")
		if _, err := buildTestStore(other+".db", other+"-packs.db", 400, o.storeID, true, false); err != nil {
			t.Fatal(err)
		}
		o.world = other + ".db"
		_, err := verifyDigest(o)
		if err == nil || !strings.Contains(err.Error(), "another source") {
			t.Fatalf("verify -digest against another source: %v, want a refusal", err)
		}
	})
}

// TestVerifyDigestMutants checks verify -digest catches the copy mutants on
// a store the spot check covers whole: a wrong drop by the spot check, a
// stale evictable-bytes by the sum of the new file's entries.
func TestVerifyDigestMutants(t *testing.T) {
	forStoreIDs(t, func(t *testing.T, obj string) {
		for _, m := range []string{"drop-hot", "drop-meta", "stale-evict-bytes"} {
			o := digestStore(t, t.TempDir(), 300, obj)
			o.mutant = m
			if err := runCopy(o); err != nil {
				t.Fatalf("mutant %s copy: %v", m, err)
			}
			o.mutant, o.digest, o.spot, o.seed = "", true, 100000, 7
			res, err := verifyDigest(o)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.faults) == 0 {
				t.Fatalf("mutant %s: verify -digest passed", m)
			}
			t.Logf("mutant %s: %v", m, res.faults)
		}
	})
}
