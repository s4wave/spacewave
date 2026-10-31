//go:build darwin || linux || windows

package kvtx

import (
	"bytes"
	"context"
	"encoding/binary"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// TestUpgradeBase58BlockKeys opens a store whose blocks are keyed in base58,
// as version 0 wrote them, with one block already moved by an interrupted
// migration, and expects every block under its binary key.
func TestUpgradeBase58BlockKeys(t *testing.T) {
	t.Run("inmem", func(t *testing.T) {
		testUpgradeBase58BlockKeys(t, store_kvtx_inmem.NewStore())
	})
	t.Run("s4db", func(t *testing.T) {
		db, err := s4db.Open(filepath.Join(t.TempDir(), "volume.s4db"), s4db.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		testUpgradeBase58BlockKeys(t, db)
	})
}

// testUpgradeBase58BlockKeys runs TestUpgradeBase58BlockKeys on one store.
func testUpgradeBase58BlockKeys(t *testing.T, store db_kvtx.Store) {
	// Prepare the blocks of the unversioned store.
	ctx := context.Background()
	keys := store_kvkey.NewDefaultKVKey()
	blocks := [][]byte{[]byte("alpha"), []byte("beta"), []byte("gamma")}
	refs := make([]*block.BlockRef, len(blocks))

	// Write version 0 blocks, leaving the last one moved.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i, data := range blocks {
		refs[i], err = block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		rm, err := refs[i].MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		key := keys.GetBlockKey(rm)
		if i != len(blocks)-1 {
			key = append(keys.GetBlockFullPrefix(), b58.Encode(rm)...)
		}
		if err := tx.Set(ctx, key, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Open the Volume, which migrates the store.
	vol, err := NewVolume(ctx, "hydra/test-volume", keys, store, nil, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	// Every block reads back.
	for i, ref := range refs {
		data, found, err := vol.GetBlock(ctx, ref)
		if err != nil || !found || !bytes.Equal(data, blocks[i]) {
			t.Fatalf("block %d: found %v data %q err %v", i, found, data, err)
		}
	}

	// Only binary block keys remain.
	rtx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	var count int
	err = rtx.ScanPrefixKeys(ctx, keys.GetBlockFullPrefix(), func(key []byte) error {
		count++
		if key[len(keys.GetBlockFullPrefix())] != binaryBlockRefTag {
			t.Errorf("block key %q is not binary", key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != len(blocks) {
		t.Fatalf("store holds %d block keys, want %d", count, len(blocks))
	}

	// The store records the current version.
	version, err := readFormatVersion(ctx, store, keys)
	if err != nil {
		t.Fatal(err)
	}
	if version != store_kvkey.FormatVersion {
		t.Fatalf("format version %d, want %d", version, store_kvkey.FormatVersion)
	}
}

// TestUpgradeCayleyRefGraph opens a version 1 store whose reference graph is
// a Cayley quad store and expects the live reference edges as edge records
// with the Cayley records deleted.
func TestUpgradeCayleyRefGraph(t *testing.T) {
	t.Run("inmem", func(t *testing.T) {
		testUpgradeCayleyRefGraph(t, store_kvtx_inmem.NewStore())
	})
	t.Run("s4db", func(t *testing.T) {
		db, err := s4db.Open(filepath.Join(t.TempDir(), "volume.s4db"), s4db.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		testUpgradeCayleyRefGraph(t, db)
	})
}

// testUpgradeCayleyRefGraph runs TestUpgradeCayleyRefGraph on one store.
func testUpgradeCayleyRefGraph(t *testing.T, store db_kvtx.Store) {
	// Migrate a version 1 store and check its graph.
	ctx := context.Background()
	keys := store_kvkey.NewDefaultKVKey()
	writeCayleyStore(t, store, keys)
	if err := upgradeFormat(ctx, store, keys); err != nil {
		t.Fatal(err)
	}
	checkConvertedRefGraph(t, store)
}

// TestUpgradeLegacyRefGraph opens a version 2 store and expects its edges in
// the current encoding with the version 2 records deleted.
func TestUpgradeLegacyRefGraph(t *testing.T) {
	// Migrate a version 2 store and check its graph.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	keys := store_kvkey.NewDefaultKVKey()
	writeLegacyStore(t, store, keys)
	if err := upgradeFormat(ctx, store, keys); err != nil {
		t.Fatal(err)
	}
	checkLegacyRefGraph(t, store)
}

// TestAdoptRefGraph opens a Volume over a store whose version 2 graph sits
// beside the Volume, as the browser engine kept it, and expects the graph in
// the Volume at the current format, the retired node inventory dropped, and
// nothing left beside it.
func TestAdoptRefGraph(t *testing.T) {
	// Write a version 2 Volume store under m/.
	ctx := context.Background()
	root := store_kvtx_inmem.NewStore()
	keys := store_kvkey.NewDefaultKVKey()
	store := kvtx_prefixer.NewPrefixer(root, []byte("m/"))
	writeLegacyStore(t, store, keys)

	// Collect the version 2 records the Volume holds.
	tx, err := root.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	inner := slices.Concat([]byte("m/"), legacyRefGraphPrefix)
	var records [][]byte
	err = tx.ScanPrefixKeys(ctx, inner, func(key []byte) error {
		records = append(records, bytes.Clone(key))
		return nil
	})
	if err != nil || len(records) == 0 {
		t.Fatalf("%d records, err %v", len(records), err)
	}

	// Move them beside the Volume, under x/, beside an inventory record of
	// the retired collector under y/.
	inventory := slices.Concat([]byte("y/n"), []byte("node"))
	if err := tx.Set(ctx, inventory, nil); err != nil {
		t.Fatal(err)
	}
	for _, key := range records {
		if err := tx.Set(ctx, slices.Concat([]byte("x/"), key[len(inner):]), nil); err != nil {
			t.Fatal(err)
		}
		if err := tx.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Adopt the graph and open the Volume, which migrates it.
	if err := AdoptRefGraph(ctx, root, []byte("m/"), []byte("x/"), []byte("y/")); err != nil {
		t.Fatal(err)
	}
	vol, err := NewVolume(ctx, "hydra/test-volume", keys, store, nil, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	checkLegacyRefGraph(t, store)
	for _, prefix := range []string{"x/", "y/", "m/" + string(refGraphPrefix) + "n"} {
		if n := countPrefix(t, root, []byte(prefix)); n != 0 {
			t.Fatalf("%d records remain under %q", n, prefix)
		}
	}
}

// TestMigrateLegacyRefGraphBatches moves more records than one batch holds,
// resuming each batch after the last, and expects every record moved.
func TestMigrateLegacyRefGraphBatches(t *testing.T) {
	// Write a version 2 root record per node across three batches.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	n := 2*convertBatchEdges + 3
	for i := range n {
		if err := tx.Set(ctx, legacyGraphKey('r', "node-"+strconv.Itoa(i)), nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Migrate and count the records under each prefix.
	if err := MigrateLegacyRefGraph(ctx, store, legacyRefGraphPrefix, refGraphPrefix); err != nil {
		t.Fatal(err)
	}
	if got := countPrefix(t, store, legacyRefGraphPrefix); got != 0 {
		t.Fatalf("%d version 2 records left", got)
	}
	if got := countPrefix(t, store, refGraphPrefix); got != n {
		t.Fatalf("moved %d records, want %d", got, n)
	}
}

// writeFormat3Graph writes edges and the graph root to store as a version 3
// store holds them: a record per direction for each edge.
func writeFormat3Graph(t *testing.T, store db_kvtx.Store, keys *store_kvkey.KVKey, edges []block_gc.RefEdge) {
	// Encode the records of the root and of each edge.
	t.Helper()
	ctx := context.Background()
	records := [][]byte{legacyGraphKey('r', block_gc.NodeGCRoot)}
	for _, edge := range edges {
		records = append(records,
			legacyGraphKey('f', block_gc.FormatIRI(edge.Subject), block_gc.FormatIRI(edge.Object)),
			legacyGraphKey('i', block_gc.FormatIRI(edge.Object), block_gc.FormatIRI(edge.Subject)),
		)
	}

	// Write them and the version in one commit.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, record := range records {
		key, err := block_gc.LegacyRecordKey(record[len(legacyRefGraphPrefix):])
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, slices.Concat(refGraphPrefix, key), nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(3)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestUpgradeChildLists opens a version 3 store, whose graph holds a record
// per direction for every edge, and expects the edges between blocks and the
// queue's block entries in child lists, the owner edges kept, and a mark that
// reaches the same blocks. The parent lists more children than one batch
// converts, so its list merges across batches.
func TestUpgradeChildLists(t *testing.T) {
	// Root a bucket owning a, and queue a garbage block listing a leaf.
	bucket := block_gc.BucketIRI("bucket")
	a, garbage, leaf := testBlockIRI(t, "a"), testBlockIRI(t, "garbage"), testBlockIRI(t, "garbage-leaf")
	edges := []block_gc.RefEdge{
		{Subject: block_gc.NodeGCRoot, Object: bucket},
		{Subject: bucket, Object: a},
		{Subject: block_gc.NodeUnreferenced, Object: garbage},
		{Subject: garbage, Object: leaf},
	}

	// Give a more children than one batch converts.
	children := make([]string, 2*convertBatchEdges+3)
	for i := range children {
		children[i] = testBlockIRI(t, "child-"+strconv.Itoa(i))
		edges = append(edges, block_gc.RefEdge{Subject: a, Object: children[i]})
	}

	// Write their version 3 records.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	keys := store_kvkey.NewDefaultKVKey()
	writeFormat3Graph(t, store, keys, edges)

	// Upgrade it.
	if err := upgradeFormat(ctx, store, keys); err != nil {
		t.Fatal(err)
	}

	// The bucket owns a, a lists every child, and each block is tracked.
	rg := openRefGraph(store)
	for node, want := range map[string][]string{
		block_gc.NodeGCRoot:       {bucket},
		bucket:                    {a},
		a:                         slices.Sorted(slices.Values(children)),
		garbage:                   {leaf},
		block_gc.NodeUnreferenced: nil,
	} {
		got, err := rg.GetOutgoingRefs(ctx, node)
		if err != nil || !slices.Equal(slices.Sorted(slices.Values(got)), want) {
			t.Fatalf("%s: outgoing %d, want %d, err %v", node, len(got), len(want), err)
		}
	}
	for node, want := range map[string][]string{
		a:           {block_gc.NodeUnreferenced, bucket},
		children[0]: {block_gc.NodeUnreferenced},
		garbage:     {block_gc.NodeUnreferenced},
		leaf:        {block_gc.NodeUnreferenced},
	} {
		got, err := rg.GetIncomingRefs(ctx, node)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("%s: incoming %v, want %v, err %v", node, got, want, err)
		}
	}

	// The mark reaches a and its children and leaves the garbage.
	mark, err := rg.MarkBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mark.Unknown != 0 || !slices.Equal(slices.Sorted(slices.Values(mark.Unreached)), slices.Sorted(slices.Values([]string{garbage, leaf}))) {
		t.Fatalf("mark: unknown %d, unreached %v", mark.Unknown, mark.Unreached)
	}
}

// countPrefix returns the number of keys under prefix.
func countPrefix(t *testing.T, store db_kvtx.Store, prefix []byte) int {
	// Scan the prefix in a snapshot.
	t.Helper()
	tx, err := store.NewTransaction(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var n int
	err = tx.ScanPrefixKeys(context.Background(), prefix, func([]byte) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// copyCase is an earlier format store CopyStore converts.
type copyCase struct {
	// version is the format version write records.
	version uint64
	// write fills the source store.
	write func(*testing.T, db_kvtx.Store, *store_kvkey.KVKey)
	// check checks the converted graph of the destination.
	check func(*testing.T, db_kvtx.Store)
	// keys and edges are the final progress counts.
	keys, edges uint64
}

// copyCases are the earlier formats CopyStore converts, by name.
var copyCases = map[string]copyCase{
	"cayley": {version: 1, write: writeCayleyStore, check: checkConvertedRefGraph, keys: 1, edges: 3},
	"legacy": {version: 2, write: writeLegacyStore, check: checkLegacyRefGraph, keys: 6, edges: 3},
}

// TestCopyStore copies a store of each earlier format and expects its other
// keys verbatim, its live reference edges in the current encoding, no earlier
// graph records and the current format version, with the source unchanged.
func TestCopyStore(t *testing.T) {
	for name, c := range copyCases {
		t.Run(name+"/inmem", func(t *testing.T) {
			testCopyStore(t, c, store_kvtx_inmem.NewStore(), store_kvtx_inmem.NewStore())
		})
		t.Run(name+"/s4db", func(t *testing.T) {
			dir := t.TempDir()
			var dbs [2]*s4db.DB
			for i, name := range []string{"src.s4db", "dst.s4db"} {
				db, err := s4db.Open(filepath.Join(dir, name), s4db.Options{})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				dbs[i] = db
			}
			testCopyStore(t, c, dbs[0], dbs[1])
		})
	}
}

// testCopyStore runs TestCopyStore on one pair of stores.
func testCopyStore(t *testing.T, c copyCase, src, dst db_kvtx.Store) {
	// Prepare an earlier format store with a key outside the graph.
	ctx := context.Background()
	keys := store_kvkey.NewDefaultKVKey()
	c.write(t, src, keys)
	tx, err := src.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("other"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Copy it.
	var last CopyProgress
	if err := CopyStore(ctx, src, dst, keys, func(p CopyProgress) { last = p }); err != nil {
		t.Fatal(err)
	}
	if last.Keys != c.keys || last.Edges != c.edges {
		t.Fatalf("progress %+v, want %d keys and %d edges", last, c.keys, c.edges)
	}

	// The destination holds the converted graph, the other key and the
	// current version, and the source keeps its version.
	c.check(t, dst)
	rtx, err := dst.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	value, found, err := rtx.Get(ctx, []byte("other"))
	if err != nil || !found || string(value) != "value" {
		t.Fatalf("other key: found %v value %q err %v", found, value, err)
	}
	for store, want := range map[db_kvtx.Store]uint64{src: c.version, dst: store_kvkey.FormatVersion} {
		version, err := readFormatVersion(ctx, store, keys)
		if err != nil {
			t.Fatal(err)
		}
		if version != want {
			t.Fatalf("format version %d, want %d", version, want)
		}
	}

	// A second copy refuses the filled destination.
	if err := CopyStore(ctx, src, dst, keys, nil); err == nil {
		t.Fatal("copy into a non-empty store succeeded")
	}
}

// writeCayleyStore writes a version 1 store whose Cayley reference graph
// holds root->a and a->b, a removed a->c, and an edge of another predicate.
func writeCayleyStore(t *testing.T, store db_kvtx.Store, keys *store_kvkey.KVKey) {
	// Open a Cayley graph in the store.
	ctx := context.Background()
	h, err := kvtx_cayley.NewGraph(ctx, kvtx_prefixer.NewPrefixer(store, cayleyGraphPrefix), graph.Options{
		"ignore_duplicate": true,
		"ignore_missing":   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add the edges, including one of another predicate, and remove one.
	ref := func(subject, object string) quad.Quad {
		return quad.MakeIRI(subject, block_gc.PredGCRef, object, "")
	}
	for _, q := range []quad.Quad{
		ref(block_gc.NodeGCRoot, "a"),
		ref("a", "b"),
		ref("a", "c"),
		quad.MakeIRI("a", "other", "d", ""),
	} {
		if err := h.AddQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.RemoveQuad(ctx, ref("a", "c")); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	// Record format version 1.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(1)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// checkConvertedRefGraph expects the edge records of the writeCayleyStore
// graph and no Cayley records.
func checkConvertedRefGraph(t *testing.T, store db_kvtx.Store) {
	// The live reference edges moved.
	ctx := context.Background()
	rg := openRefGraph(store)
	for subject, want := range map[string][]string{
		block_gc.NodeGCRoot: {"a"},
		"a":                 {"b"},
	} {
		got, err := rg.GetOutgoingRefs(ctx, subject)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("outgoing refs of %q: %v, want %v", subject, got, want)
		}
	}

	// No Cayley records remain.
	rtx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	err = rtx.ScanPrefixKeys(ctx, cayleyGraphPrefix, func(key []byte) error {
		t.Errorf("Cayley record %q remains", key)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// writeLegacyStore writes a version 2 store whose graph holds gcroot->bucket,
// bucket->a and a->b, with a and b block nodes spelled in base58 as version 2
// stored them, and registers gcroot as a root.
func writeLegacyStore(t *testing.T, store db_kvtx.Store, keys *store_kvkey.KVKey) {
	// Collect the root record and both records of each edge.
	records := [][]byte{legacyGraphKey('r', block_gc.NodeGCRoot)}
	for _, edge := range legacyEdges(t) {
		records = append(records,
			legacyGraphKey('f', block_gc.FormatIRI(edge.Subject), block_gc.FormatIRI(edge.Object)),
			legacyGraphKey('i', block_gc.FormatIRI(edge.Object), block_gc.FormatIRI(edge.Subject)),
		)
	}

	// Write them with the format version.
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, key := range records {
		if err := tx.Set(ctx, key, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(2)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// legacyGraphKey encodes a version 2 record key.
func legacyGraphKey(kind byte, nodes ...string) []byte {
	key := append(bytes.Clone(legacyRefGraphPrefix), kind)
	for _, node := range nodes {
		key = binary.BigEndian.AppendUint32(key, uint32(len(node))) //nolint:gosec // test IRIs are short.
		key = append(key, node...)
	}
	return key
}

// legacyEdges returns the edges writeLegacyStore writes.
func legacyEdges(t *testing.T) []block_gc.RefEdge {
	t.Helper()
	bucket := block_gc.BucketIRI("bucket")
	a, b := testBlockIRI(t, "a"), testBlockIRI(t, "b")
	return []block_gc.RefEdge{
		{Subject: block_gc.NodeGCRoot, Object: bucket},
		{Subject: bucket, Object: a},
		{Subject: a, Object: b},
	}
}

// testBlockIRI returns the IRI of a block holding data.
func testBlockIRI(t *testing.T, data string) string {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return block_gc.BlockIRI(block.NewBlockRef(h))
}

// checkLegacyRefGraph expects the writeLegacyStore edges forward, each owner
// that is not a block and the staging edge of each tracked block back, its
// root and no version 2 records.
func checkLegacyRefGraph(t *testing.T, store db_kvtx.Store) {
	// Each edge reads forward and back.
	ctx := context.Background()
	rg := openRefGraph(store)
	for _, edge := range legacyEdges(t) {
		out, err := rg.GetOutgoingRefs(ctx, edge.Subject)
		if err != nil {
			t.Fatal(err)
		}
		in, err := rg.GetIncomingRefs(ctx, edge.Object)
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		if _, ok := block_gc.ParseBlockIRI(edge.Object); ok {
			want = append(want, block_gc.NodeUnreferenced)
		}
		if _, ok := block_gc.ParseBlockIRI(edge.Subject); !ok {
			want = append(want, edge.Subject)
		}
		if !slices.Equal(out, []string{edge.Object}) || !slices.Equal(in, want) {
			t.Fatalf("edge %v: outgoing %v, incoming %v", edge, out, in)
		}
	}

	// The root survives.
	roots, err := block_gc.NewRefGraph(kvtx_prefixer.NewPrefixer(store, refGraphPrefix)).GetRootNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(roots, []string{block_gc.NodeGCRoot}) {
		t.Fatalf("roots %v", roots)
	}

	// No version 2 records remain.
	rtx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	err = rtx.ScanPrefixKeys(ctx, legacyRefGraphPrefix, func(key []byte) error {
		t.Errorf("version 2 record %q remains", key)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCopyReachable copies a store whose graph root reaches blocks a and b
// through a bucket, with b also left in the unreferenced queue, and whose
// registered root owns block f, while c sits only in the queue, d has no graph
// record and e belongs to a bucket nothing reaches. f has no child list, so
// the copy refuses the source before writing anything until unknown blocks
// are allowed. The copy then keeps a, b, f, their records and the key outside
// the graph, and drops the rest.
func TestCopyReachable(t *testing.T) {
	t.Run("inmem", func(t *testing.T) {
		testCopyReachable(t, store_kvtx_inmem.NewStore(), store_kvtx_inmem.NewStore())
	})
	t.Run("s4db", func(t *testing.T) {
		dir := t.TempDir()
		var dbs [2]*s4db.DB
		for i, name := range []string{"src.s4db", "dst.s4db"} {
			db, err := s4db.Open(filepath.Join(dir, name), s4db.Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			dbs[i] = db
		}
		testCopyReachable(t, dbs[0], dbs[1])
	})
}

// testCopyReachable runs TestCopyReachable on one pair of stores.
func testCopyReachable(t *testing.T, src, dst db_kvtx.Store) {
	// Open a write transaction on the source.
	ctx := context.Background()
	keys := store_kvkey.NewDefaultKVKey()
	tx, err := src.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Write the blocks.
	refs := make(map[string]*block.BlockRef)
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		ref, err := block.BuildBlockRef([]byte(name), nil)
		if err != nil {
			t.Fatal(err)
		}
		rm, err := ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, keys.GetBlockKey(rm), []byte(name)); err != nil {
			t.Fatal(err)
		}
		refs[name] = ref
	}

	// Write the key outside the graph and the current format version.
	if err := tx.Set(ctx, []byte("other"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(store_kvkey.FormatVersion)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Write the graph.
	iri := func(name string) string { return block_gc.BlockIRI(refs[name]) }
	graph := openRefGraph(src)
	if err := graph.AddRoot(ctx, block_gc.BucketIRI("pinned")); err != nil {
		t.Fatal(err)
	}
	err = graph.ApplyRefBatch(ctx, []block_gc.RefEdge{
		{Subject: block_gc.NodeGCRoot, Object: block_gc.BucketIRI("live")},
		{Subject: block_gc.BucketIRI("live"), Object: iri("a")},
		{Subject: iri("a"), Object: iri("b")},
		{Subject: block_gc.NodeUnreferenced, Object: iri("b")},
		{Subject: block_gc.NodeUnreferenced, Object: iri("c")},
		{Subject: block_gc.BucketIRI("gone"), Object: iri("e")},
		{Subject: block_gc.BucketIRI("pinned"), Object: iri("f")},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The copy refuses the unknown block f and writes nothing.
	if err := CopyReachable(ctx, src, dst, keys, CopyReachableOptions{}, nil); err == nil {
		t.Fatal("reachable copy past an unknown block succeeded")
	}
	if err := requireEmptyStore(ctx, dst); err != nil {
		t.Fatal(err)
	}

	// Copy the reachable part, allowing it.
	var last CopyProgress
	opts := CopyReachableOptions{AllowUnknown: true, CountUnowned: true}
	if err := CopyReachable(ctx, src, dst, keys, opts, func(p CopyProgress) { last = p }); err != nil {
		t.Fatal(err)
	}

	// An owner edge is two records, an edge between blocks or from the queue
	// lands in its block's child list, and a registered root has one root
	// record: the copy keeps the reachable edges and lists and the root's
	// record, and drops c's list, the unreachable bucket's edge and the young
	// log the writes appended.
	want := CopyProgress{Keys: 13, UnknownBlocks: 1, DroppedBlocks: 3, UnownedBlocks: 1, DroppedRecords: 5}
	last.Bytes, last.DroppedBytes = 0, 0
	if last != want {
		t.Fatalf("progress %+v, want %+v", last, want)
	}

	// The destination holds the reachable blocks and edges and the other key.
	rtx, err := dst.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	for name, kept := range map[string]bool{"a": true, "b": true, "c": false, "d": false, "e": false, "f": true} {
		rm, err := refs[name].MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		found, err := rtx.Exists(ctx, keys.GetBlockKey(rm))
		if err != nil || found != kept {
			t.Fatalf("block %s: found %v, want %v, err %v", name, found, kept, err)
		}
	}
	if found, err := rtx.Exists(ctx, []byte("other")); err != nil || !found {
		t.Fatalf("other key: found %v err %v", found, err)
	}
	rg := openRefGraph(dst)
	for node, want := range map[string][]string{
		block_gc.NodeGCRoot:          {block_gc.BucketIRI("live")},
		block_gc.BucketIRI("live"):   {iri("a")},
		iri("a"):                     {iri("b")},
		block_gc.NodeUnreferenced:    nil,
		block_gc.BucketIRI("gone"):   nil,
		block_gc.BucketIRI("pinned"): {iri("f")},
	} {
		got, err := rg.GetOutgoingRefs(ctx, node)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("outgoing refs of %s: %v, want %v", node, got, want)
		}
	}

	// A store at an earlier format is refused.
	old := store_kvtx_inmem.NewStore()
	writeLegacyStore(t, old, keys)
	if err := CopyReachable(ctx, old, store_kvtx_inmem.NewStore(), keys, opts, nil); err == nil {
		t.Fatal("reachable copy of a version 2 store succeeded")
	}
}

// TestCopyStoreDropping copies a current store, leaving out the blocks the
// drop check reports, and expects the other blocks and keys verbatim.
func TestCopyStoreDropping(t *testing.T) {
	t.Run("inmem", func(t *testing.T) {
		testCopyStoreDropping(t, store_kvtx_inmem.NewStore(), store_kvtx_inmem.NewStore())
	})
	t.Run("s4db", func(t *testing.T) {
		dir := t.TempDir()
		var dbs [2]*s4db.DB
		for i, name := range []string{"src.s4db", "dst.s4db"} {
			db, err := s4db.Open(filepath.Join(dir, name), s4db.Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			dbs[i] = db
		}
		testCopyStoreDropping(t, dbs[0], dbs[1])
	})
}

// testCopyStoreDropping runs TestCopyStoreDropping on one pair of stores.
func testCopyStoreDropping(t *testing.T, src, dst db_kvtx.Store) {
	// Write blocks a to e, of which b and d are dropped, around keys outside
	// the blocks, at the current format.
	ctx := context.Background()
	keys := store_kvkey.NewDefaultKVKey()
	blockKeys := make(map[string][]byte)
	tx, err := src.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(name))
		if err != nil {
			t.Fatal(err)
		}
		rm, err := block.NewBlockRef(h).MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		blockKeys[name] = keys.GetBlockKey(rm)
		if err := tx.Set(ctx, blockKeys[name], []byte("value "+name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"\x00first", "other", "\xffzz"} {
		if err := tx.Set(ctx, []byte(key), []byte("value")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(store_kvkey.FormatVersion)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	dropped := map[string]bool{"b": true, "d": true}
	dropHashes := make(map[string]bool)
	for name := range dropped {
		h, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte(name))
		dropHashes[h.MarshalString()] = true
	}

	// Copy it, dropping b and d.
	var checked int
	drop := func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
		checked += len(refs)
		out := make([]bool, len(refs))
		for i, ref := range refs {
			out[i] = dropHashes[ref.GetHash().MarshalString()]
		}
		return out, nil
	}
	var last CopyProgress
	if err := CopyStoreDropping(ctx, src, dst, keys, drop, func(p CopyProgress) { last = p }); err != nil {
		t.Fatal(err)
	}
	if checked != 5 || last.Keys != 6 || last.DroppedBlocks != 2 {
		t.Fatalf("checked %d blocks, progress %+v; want 5 checked, 6 keys and 2 dropped blocks", checked, last)
	}

	// The destination holds the kept blocks and other keys, and the version.
	rtx, err := dst.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	for name, key := range blockKeys {
		value, found, err := rtx.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if found == dropped[name] || (found && string(value) != "value "+name) {
			t.Fatalf("block %s: found %v value %q, dropped %v", name, found, value, dropped[name])
		}
	}
	for _, key := range []string{"\x00first", "other", "\xffzz"} {
		if value, found, err := rtx.Get(ctx, []byte(key)); err != nil || !found || string(value) != "value" {
			t.Fatalf("key %q: found %v value %q err %v", key, found, value, err)
		}
	}
	if version, err := readFormatVersion(ctx, dst, keys); err != nil || version != store_kvkey.FormatVersion {
		t.Fatalf("format version %d err %v", version, err)
	}
}
