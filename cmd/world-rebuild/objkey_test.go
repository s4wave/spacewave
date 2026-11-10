package main

import (
	"crypto/sha256"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// prodObj is an object store prefix shaped like production's write-back
// store id, p/<provider>/<account>/blk/<blockstore>/writeback.
const prodObj = testObjPrefix

// testB58 returns the b58 hash string and digest of a test block.
func testB58(s string) (string, string) {
	d := sha256.Sum256([]byte(s))
	h := hash.NewHash(hash.HashType_HashType_SHA256, d[:])
	return h.MarshalString(), string(d[:])
}

// TestObjKeySlashedID checks the object store id is split at the sub-key
// boundary, not at the first slash, for production-shaped ids.
func TestObjKeySlashedID(t *testing.T) {
	b58, _ := testB58("a")
	stamp := "0000000000000003"
	for _, c := range []struct {
		key, obj, rest string
	}{
		{prodObj + "marked/" + b58, "p/local/acct1/blk/hyperline-world/writeback", "marked/" + b58},
		{prodObj + "evictable/" + stamp + "/" + b58, "p/local/acct1/blk/hyperline-world/writeback", "evictable/" + stamp + "/" + b58},
		{prodObj + "queue/" + stamp + "/" + b58, "p/local/acct1/blk/hyperline-world/writeback", "queue/" + stamp + "/" + b58},
		{prodObj + "reads/" + stamp + "/" + stamp, "p/local/acct1/blk/hyperline-world/writeback", "reads/" + stamp + "/" + stamp},
		{prodObj + "evictable-bytes", "p/local/acct1/blk/hyperline-world/writeback", "evictable-bytes"},
		{prodObj + "epoch", "p/local/acct1/blk/hyperline-world/writeback", "epoch"},
		{prodObj + "queue-count", "p/local/acct1/blk/hyperline-world/writeback", "queue-count"},
		{prodObj + "backfill", "p/local/acct1/blk/hyperline-world/writeback", "backfill"},
		{prodObj + "copy/remote-1", "p/local/acct1/blk/hyperline-world/writeback", "copy/remote-1"},
		{prodObj + "copy-done/remote-1", "p/local/acct1/blk/hyperline-world/writeback", "copy-done/remote-1"},
		{"h/objs/writeback/marked/" + b58, "writeback", "marked/" + b58},
		{"h/objs/writeback/evictable-bytes", "writeback", "evictable-bytes"},
		// A sub-key name inside the id: the shape of the rest decides.
		{"h/objs/p/marked/writeback/x/writeback/marked/" + b58, "p/marked/writeback/x/writeback", "marked/" + b58},
	} {
		obj, rest, ok := objKey([]byte(c.key))
		if !ok || obj != c.obj || rest != c.rest {
			t.Errorf("%s: got %q %q %v, want %q %q", c.key, obj, rest, ok, c.obj, c.rest)
		}
	}
	// Unknown and ambiguous layouts do not split, and parse as bad.
	for _, key := range []string{
		prodObj + "unknown-thing",
		"h/objs/writeback/x/marked/" + b58,          // writeback segment, unknown layout
		"h/objs/a/writeback/copy/b/writeback/epoch", // two boundaries
		"h/objs/writeback",
		prodObj + "evictable/" + b58, // no epoch
	} {
		if obj, rest, ok := objKey([]byte(key)); ok {
			t.Errorf("%s: split as %q %q, want refused", key, obj, rest)
		}
		if k := objKeyOf([]byte(key)); !k.ok || k.kind != kindBad {
			t.Errorf("%s: objKeyOf %+v, want kindBad", key, k)
		}
	}
	// Keys of object stores that are not write-back stores are foreign:
	// kept, with no drop fact, even where a segment looks like a sub-key.
	for _, key := range []string{
		"h/objs/noslash",
		"h/objs/marked/" + b58,
		"h/objs/p/local/acct1/sess/acct1/pk",
		"h/objs/so/so-binding/account-settings",
		"h/objs/so/so-list",
		"h/objs/so/so/abc/host",
		"h/objs/so/so/abc/config-history/checkpoint",
		"h/objs/so/so/abc/ls/world-replay/cursor",
		"h/objs/so/x/marked/" + b58,
		"h/objs/so/x/evictable/0000000000000003/" + b58,
	} {
		if k := objKeyOf([]byte(key)); !k.ok || k.kind != kindForeign {
			t.Errorf("%s: objKeyOf %+v, want kindForeign", key, k)
		}
	}
}

// TestFactsSlashedID checks the drop rule sees the markers, eviction entries
// and read set of a production-shaped store, and of one without slashes in
// the same World.
func TestFactsSlashedID(t *testing.T) {
	stamp := "0000000000000003"
	for _, obj := range []string{prodObj, "h/objs/writeback/"} {
		markedB58, markedD := testB58(obj + "marked")
		hotB58, hotD := testB58(obj + "hot")
		dropB58, dropD := testB58(obj + "drop")
		keys := []string{
			obj + "marked/" + markedB58,
			obj + "evictable/" + stamp + "/" + markedB58,
			obj + "evictable/" + stamp + "/" + hotB58,
			obj + "evictable/" + stamp + "/" + dropB58,
		}
		f := &facts{marked: map[string]struct{}{}, evict: map[string][]evictEntry{}, packed: map[string]struct{}{}, hot: map[[readKeyLen]byte]struct{}{}}
		for _, key := range keys {
			k := objKeyOf([]byte(key))
			switch k.kind {
			case kindMarked:
				f.marked[k.digest] = struct{}{}
			case kindEvict:
				f.evict[k.digest] = append(f.evict[k.digest], evictEntry{obj: k.obj})
				f.packed[k.digest] = struct{}{}
			default:
				t.Fatalf("%s: kind %d", key, k.kind)
			}
		}
		if k := objKeyOf([]byte(obj + "reads/" + stamp + "/" + stamp)); k.kind != kindReads || k.gen != stamp {
			t.Fatalf("reads key: %+v", k)
		}
		var hk [readKeyLen]byte
		copy(hk[:], hotD)
		f.hot[hk] = struct{}{}
		for _, c := range []struct{ d, want string }{
			{markedD, vMarkedEvic},
			{hotD, vHot},
			{dropD, vDrop},
		} {
			if got := f.verdict(c.d); got != c.want {
				t.Errorf("%s: verdict %q, want %q", obj, got, c.want)
			}
		}
	}
}

// TestCopyRefusesUnknownObjKey checks copy and count refuse a World holding
// an object store key they cannot classify, rather than reading it as other.
func TestCopyRefusesUnknownObjKey(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "raw", 20000
	o.walMax = 2560
	if _, err := buildTestStore(o.world, o.packs, 200, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", o.world)
	if err != nil {
		t.Fatal(err)
	}
	// A marker under a layout the parser does not know.
	b58, _ := testB58("unknown")
	if _, err := db.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte(testObjPrefix+"marked-v2/"+b58), []byte{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := runCopy(o); err == nil {
		t.Fatal("copy of a World with an unclassifiable object store key succeeded")
	}
	if _, err := runCount(o); err == nil {
		t.Fatal("count of a World with an unclassifiable object store key succeeded")
	}
}

// TestCountProdShapedVerdicts checks count on the production-shaped test
// store keeps the marked and hot blocks and drops exactly the cold, packed,
// queued ones.
func TestCountProdShapedVerdicts(t *testing.T) {
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.storeID, o.chunk, o.walMax = defaultStoreID, 500, 2560
	const n = 400
	want, err := buildTestStore(o.world, o.packs, n, o.storeID, true, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := runCount(o)
	if err != nil {
		t.Fatal(err)
	}
	get := func(v string) int64 {
		if t := res.blocks[v]; t != nil {
			return t.keys
		}
		return 0
	}
	if get(vDrop) != want || want == 0 {
		t.Errorf("drop %d, want %d", get(vDrop), want)
	}
	if get(vMarked) != n/10 {
		t.Errorf("marked kept %d, want %d", get(vMarked), n/10)
	}
	if get(vHot) != n/10 {
		t.Errorf("hot kept %d, want %d", get(vHot), n/10)
	}
	if bad, _ := res.census.bad(); bad != 0 {
		t.Errorf("census: %d bad rows", bad)
	}
}

// sharedRefWorld builds the test store and adds rows of a second write-back
// store, 01m6…, of the same volume, for a block ref hyperline-world drops.
func sharedRefWorld(t *testing.T, rows func(other, b58 string) []string) opts {
	t.Helper()
	dir := t.TempDir()
	var o opts
	o.world = filepath.Join(dir, "world.db")
	o.packs = filepath.Join(dir, "packs.db")
	o.out = filepath.Join(dir, "new.db")
	o.storeID, o.chunk, o.readers, o.read, o.txnRows = defaultStoreID, 500, 2, "raw", 20000
	o.walMax = 2560
	if _, err := buildTestStore(o.world, o.packs, 200, o.storeID, true, false); err != nil {
		t.Fatal(err)
	}

	// Find a block the rule drops: its eviction entry in hyperline-world.
	db, err := sql.Open("sqlite", o.world)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f, err := loadFacts(o, db, "kv", mustOpen(t, o.packs))
	if err != nil {
		t.Fatal(err)
	}
	var b58 string
	for d := range f.evict {
		if f.verdict(d) == vDrop {
			b58 = hash.NewHash(f.hashTypes[d], []byte(d)).MarshalString()
			break
		}
	}
	if b58 == "" {
		t.Fatal("no dropped block")
	}

	// Write the second store's rows for it.
	other := "h/objs/p/local/acct1/blk/01m6wbbw9t9z0vv0c4t5spvs68/writeback/"
	for _, k := range rows(other, b58) {
		if _, err := db.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []byte(k), []byte("10")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	return o
}

// mustOpen opens a sqlite file for the test.
func mustOpen(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestCopyRefusesOtherStoreFacts checks copy and count refuse a World where
// another write-back store of the volume marks or queues for eviction a
// block ref hyperline-world would drop: h/b/ is shared by every store.
func TestCopyRefusesOtherStoreFacts(t *testing.T) {
	for name, rows := range map[string]func(other, b58 string) []string{
		"marked":    func(other, b58 string) []string { return []string{other + "marked/" + b58} },
		"evictable": func(other, b58 string) []string { return []string{other + "evictable/0000000000000000/" + b58} },
	} {
		t.Run(name, func(t *testing.T) {
			o := sharedRefWorld(t, rows)
			if err := runCopy(o); err == nil {
				t.Fatal("copy succeeded with another store's " + name + " row for a dropped ref")
			}
			if _, err := runCount(o); err == nil {
				t.Fatal("count succeeded with another store's " + name + " row for a dropped ref")
			}
		})
	}

	// A store holding only its queue count, as production's other stores
	// do, is no bar.
	o := sharedRefWorld(t, func(other, _ string) []string { return []string{other + "queue-count"} })
	if err := runCopy(o); err != nil {
		t.Fatal("copy with another store's queue-count:", err)
	}
}
