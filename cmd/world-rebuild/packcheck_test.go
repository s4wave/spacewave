package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
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
	r, err := p.Exec("DELETE FROM kv WHERE key = ?", append(block_store_s3.PackIndexPrefix(o.storeID), append([]byte("h"), md[len(md)-indexKeyBytes:]...)...))
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

// TestPackCheckNeededFaults checks packcheck -needed counts a pack with no
// e/ row (size -1) and the noRow and deadPack counts of the file's header as
// faults, and still checks the -1 pack is listed.
func TestPackCheckNeededFaults(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "list.txt")
	writeList(t, list, "100;packs/packA", "10;entries/packA", "10;entries/packB")
	needed := func(name string, lines ...string) string {
		p := filepath.Join(dir, name)
		writeList(t, p, lines...)
		return p
	}

	// The clean file passes.
	if _, err := runPackCheck(opts{needed: needed("ok.txt", "# noRow 0 deadPack 0", "packA 100 3"), list: list}); err != nil {
		t.Fatal("clean needed file:", err)
	}

	// A -1 pack is a fault, and is checked against the listing.
	res, err := runPackCheck(opts{needed: needed("noentry.txt", "# noRow 0 deadPack 0", "packA 100 3", "packB -1 2"), list: list})
	if err == nil || res.noEntry != 1 || res.missing != 1 {
		t.Fatalf("-1 pack: err %v, %+v, want a fault with 1 noEntry and 1 missing", err, res)
	}
	res, err = runPackCheck(opts{needed: needed("noentry-listed.txt", "# noRow 0 deadPack 0", "packA -1 3"), list: list})
	if err == nil || res.noEntry != 1 || res.missing != 0 {
		t.Fatalf("listed -1 pack: err %v, %+v, want a fault with 1 noEntry and none missing", err, res)
	}

	// The header's counts are faults.
	res, err = runPackCheck(opts{needed: needed("norow.txt", "# noRow 2 deadPack 0", "packA 100 3"), list: list})
	if err == nil || res.noRow != 2 {
		t.Fatalf("noRow header: err %v, %+v, want a fault with 2 noRow", err, res)
	}
	res, err = runPackCheck(opts{needed: needed("dead.txt", "# noRow 0 deadPack 1", "packA 100 3"), list: list})
	if err == nil || res.deadPack != 1 {
		t.Fatalf("deadPack header: err %v, %+v, want a fault with 1 deadPack", err, res)
	}

	// A file with no header is refused.
	if _, err := runPackCheck(opts{needed: needed("old.txt", "packA 100 3"), list: list}); err == nil {
		t.Fatal("needed file with no header passed")
	}
}

// TestPackCheckListing checks -list-prefix takes only packs/ and entries/
// directly under the prefix, and that a listing with no entries/ objects is
// a fault unless -no-entries is given.
func TestPackCheckListing(t *testing.T) {
	dir := t.TempDir()
	nd := filepath.Join(dir, "needed.txt")
	writeList(t, nd, "# noRow 0 deadPack 0", "packA 100 3")
	list := filepath.Join(dir, "list.txt")

	// A same-id object under another prefix does not count.
	writeList(t, list, "100;other-store/packs/packA", "10;hyperline-world/entries/packA")
	res, err := runPackCheck(opts{needed: nd, list: list, listPrefix: "hyperline-world/"})
	if err == nil || res.missing != 1 {
		t.Fatalf("pack under another prefix: err %v, %+v, want 1 missing", err, res)
	}
	writeList(t, list, "100;hyperline-world/packs/packA", "10;other/hyperline-world/entries/packA", "10;hyperline-world/entries/packA")
	if _, err := runPackCheck(opts{needed: nd, list: list, listPrefix: "hyperline-world/"}); err != nil {
		t.Fatal("pack under the prefix:", err)
	}

	// No entries/ objects at all is a fault, unless -no-entries.
	writeList(t, list, "100;packs/packA")
	if _, err := runPackCheck(opts{needed: nd, list: list}); err == nil {
		t.Fatal("listing with no entries/ passed")
	}
	if _, err := runPackCheck(opts{needed: nd, list: list, noEntries: true}); err != nil {
		t.Fatal("listing with no entries/ and -no-entries:", err)
	}
}
