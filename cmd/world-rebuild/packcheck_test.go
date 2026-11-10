package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// writeList writes a listing file of the lines.
func writeList(t *testing.T, path string, lines ...string) {
	t.Helper()
	var b []byte
	for _, l := range lines {
		b = append(b, l+"\n"...)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPackCheck copies the selftest store, then checks the packs of the
// blocks the copy dropped against listings: a full one passes, a missing
// pack, a short pack and a dropped block with no h row fail, and a rerun
// from the saved needed list alone gives the same answer.
func TestPackCheck(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "raw", 20000
	drop, err := buildTestStore(o.world, o.packs, 600, o.storeID, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := runCopy(o); err != nil {
		t.Fatal(err)
	}
	list := func(name string, lines ...string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		var b []byte
		for _, l := range lines {
			b = append(b, l+"\n"...)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b := testPackSizes[0], testPackSizes[1]
	good := list("good.txt",
		fmt.Sprintf("%d;p/packs/packA", a), fmt.Sprintf("%d;p/packs/packB", b),
		"300;p/entries/packA", "300;p/entries/packB", "5;p/reclaim/packZ", "77;p/packs/other")
	check := func(name string, po opts, wantErr bool) packCheckResult {
		t.Helper()
		res, err := runPackCheck(po)
		if (err != nil) != wantErr {
			t.Fatalf("%s: err %v, want error %v (%+v)", name, err, wantErr, res)
		}
		return res
	}

	o.list = good
	o.neededOut = filepath.Join(dir, "needed.txt")
	res := check("good listing", o, false)
	if res.dropped != drop || res.needed != 2 || res.missing+res.mismatch+res.noRow+res.deadPack+res.noEntry+res.entriesMissing != 0 {
		t.Fatalf("good listing: %+v, want %d dropped in 2 packs and no faults", res, drop)
	}

	// The rerun reads only the needed list and a listing.
	ro := opts{needed: o.neededOut, list: good}
	if res := check("rerun", ro, false); res.needed != 2 {
		t.Fatalf("rerun: %+v, want 2 packs", res)
	}
	ro.list = list("nob.txt", fmt.Sprintf("%d;p/packs/packA", a))
	if res := check("rerun, packB gone", ro, true); res.missing != 1 {
		t.Fatalf("rerun, packB gone: %+v, want 1 missing", res)
	}

	o.neededOut = ""
	o.list = list("missing.txt", fmt.Sprintf("%d;p/packs/packA", a), "300;p/entries/packA", "300;p/entries/packB")
	if res := check("packB missing", o, true); res.missing != 1 {
		t.Fatalf("packB missing: %+v, want 1 missing", res)
	}
	o.list = list("short.txt", fmt.Sprintf("%d;p/packs/packA", a-1), fmt.Sprintf("%d;p/packs/packB", b))
	if res := check("packA short", o, true); res.mismatch != 1 {
		t.Fatalf("packA short: %+v, want 1 size mismatch", res)
	}
	o.list = list("noentry.txt", fmt.Sprintf("%d;p/packs/packA", a), fmt.Sprintf("%d;p/packs/packB", b), "300;p/entries/packA")
	if res := check("packB entry missing", o, true); res.entriesMissing != 1 {
		t.Fatalf("packB entry missing: %+v, want 1 entry missing", res)
	}

	// A dropped block whose h row is gone is a fault. Block 5 is in the
	// dropped category of the test store.
	digest := sha256.Sum256([]byte("block-5"))
	h := hash.NewHash(hash.HashType_HashType_SHA256, digest[:])
	md := h.MarshalDigest()
	p, err := sql.Open("sqlite", "file:"+o.packs)
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Exec("DELETE FROM kv WHERE key = ?", append(packIndexPrefix(o.storeID), append([]byte("h"), md[len(md)-indexKeyBytes:]...)...))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := r.RowsAffected(); n != 1 {
		t.Fatalf("deleted %d h rows, want 1", n)
	}
	p.Close()
	o.list = good
	if res := check("h row gone", o, true); res.noRow != 1 {
		t.Fatalf("h row gone: %+v, want 1 dropped block with no h row", res)
	}
}
