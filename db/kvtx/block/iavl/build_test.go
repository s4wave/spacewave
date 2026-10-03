package kvtx_block_iavl

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"os"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// buildTestEnv creates a testbed and returns it with a bucket lookup cursor.
func buildTestEnv(t *testing.T) (*testbed.Testbed, *bucket_lookup.Cursor) {
	// Prepare the helper context and testbed logger.
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the testbed that will store the tree blocks.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty bucket cursor for the tree.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	return tb, oc
}

// writeValue writes raw bytes to the block store and returns the BlockRef.
func writeValue(t *testing.T, store block.StoreOps, val []byte) *block.BlockRef {
	t.Helper()
	ref, _, err := store.PutBlock(context.Background(), val, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

// sortedEntries returns an iterator over sorted (key, BlockRef) pairs.
func sortedEntries(keys [][]byte, refs []*block.BlockRef) iter.Seq2[[]byte, *block.BlockRef] {
	return func(yield func([]byte, *block.BlockRef) bool) {
		for i, k := range keys {
			if !yield(k, refs[i]) {
				return
			}
		}
	}
}

// readTreeViaAVL writes the built tree, creates an AVLTree from the root, and
// returns a read-only Tx. Caller must Discard.
func readTreeViaAVL(t *testing.T, oc *bucket_lookup.Cursor, rootRef *block.BlockRef) *Tx {
	// Follow the persisted root through the bucket cursor.
	t.Helper()
	ctx := context.Background()
	objRef := &bucket.ObjectRef{RootRef: rootRef}
	fc, err := oc.FollowRef(ctx, objRef)
	if err != nil {
		t.Fatal(err)
	}

	// Open a read transaction and register its cleanup.
	tr := NewAVLTree(fc)
	atx, err := tr.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { atx.Discard() })
	return atx
}

func TestBuildTreeEmpty(t *testing.T) {
	// Prepare a testbed and an iterator with no entries.
	tb, _ := buildTestEnv(t)
	empty := func(yield func([]byte, *block.BlockRef) bool) {}

	// Build the empty tree and check construction succeeds.
	tx, cs, err := BuildTree(tb.Volume, nil, nil, empty)
	if err != nil {
		t.Fatal(err)
	}

	// Verify an empty iterator produces no transaction or cursor.
	if tx != nil || cs != nil {
		t.Fatal("expected nil for empty iterator")
	}
}

func TestBuildTreeSingle(t *testing.T) {
	// Start the testbed for a single-leaf tree.
	ctx := context.Background()
	tb, oc := buildTestEnv(t)

	// Persist the value and prepare its sorted entry.
	valRef := writeValue(t, tb.Volume, []byte("value-a"))
	keys := [][]byte{[]byte("key-a")}
	refs := []*block.BlockRef{valRef}

	// Build a tree containing the single entry.
	tx, _, err := BuildTree(tb.Volume, nil, nil, sortedEntries(keys, refs))
	if err != nil {
		t.Fatal(err)
	}

	// Persist the constructed tree and report its root.
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("root ref: %v", rootRef)

	// Verify the persisted tree contains one entry.
	atx := readTreeViaAVL(t, oc, rootRef)
	sz, err := atx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sz != 1 {
		t.Fatalf("expected size 1, got %d", sz)
	}

	// Verify the persisted leaf is reachable by its key.
	exists, err := atx.Exists(ctx, []byte("key-a"))
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("key-a not found")
	}
}

func TestBuildTreeTwo(t *testing.T) {
	// Start the testbed for a two-leaf tree.
	ctx := context.Background()
	tb, oc := buildTestEnv(t)

	// Persist the values and prepare their sorted entries.
	keys := [][]byte{[]byte("a"), []byte("b")}
	refs := []*block.BlockRef{
		writeValue(t, tb.Volume, []byte("val-a")),
		writeValue(t, tb.Volume, []byte("val-b")),
	}

	// Build a tree containing both entries.
	tx, _, err := BuildTree(tb.Volume, nil, nil, sortedEntries(keys, refs))
	if err != nil {
		t.Fatal(err)
	}

	// Persist the constructed two-leaf tree.
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the persisted tree contains both leaves.
	atx := readTreeViaAVL(t, oc, rootRef)
	sz, err := atx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sz != 2 {
		t.Fatalf("expected size 2, got %d", sz)
	}

	// Verify each input key is reachable in the persisted tree.
	for _, k := range keys {
		exists, err := atx.Exists(ctx, k)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("key %s not found", k)
		}
	}
}

func TestBuildTree100(t *testing.T) {
	// Start the testbed for a hundred-leaf tree.
	ctx := context.Background()
	tb, oc := buildTestEnv(t)

	// Prepare sorted keys and persist their values.
	n := 100
	keys := make([][]byte, n)
	refs := make([]*block.BlockRef, n)
	for i := range n {
		keys[i] = fmt.Appendf(nil, "key-%04d", i)
		refs[i] = writeValue(t, tb.Volume, fmt.Appendf(nil, "val-%04d", i))
	}

	// Build the balanced tree from the sorted entries.
	tx, _, err := BuildTree(tb.Volume, nil, nil, sortedEntries(keys, refs))
	if err != nil {
		t.Fatal(err)
	}

	// Persist the constructed hundred-leaf tree.
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Print tree structure.
	objRef := &bucket.ObjectRef{RootRef: rootRef}
	fc, err := oc.FollowRef(ctx, objRef)
	if err != nil {
		t.Fatal(err)
	}
	tr := NewAVLTree(fc)
	treeStr, err := PrintIAVLTree(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr.WriteString(treeStr)

	// Open the persisted tree for readback checks.
	atx := readTreeViaAVL(t, oc, rootRef)

	// Verify size.
	sz, err := atx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if int(sz) != n { //nolint:gosec
		t.Fatalf("expected size %d, got %d", n, sz)
	}

	// Verify all keys exist.
	for i := range n {
		key := fmt.Appendf(nil, "key-%04d", i)
		exists, err := atx.Exists(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("key %s not found", key)
		}
	}

	// Verify forward iteration matches input order.
	it := atx.Iterate(ctx, nil, true, false)
	defer it.Close()
	idx := 0
	for it.Next() && it.Valid() {
		expected := fmt.Appendf(nil, "key-%04d", idx)
		if !bytes.Equal(it.Key(), expected) {
			t.Fatalf("iteration %d: expected %s, got %s", idx, expected, it.Key())
		}
		idx++
	}
	if idx != n {
		t.Fatalf("iterated %d keys, expected %d", idx, n)
	}

	// Verify height is reasonable: ceil(log2(n)) for balanced tree.
	h := atx.root.GetHeight()
	t.Logf("tree height: %d (n=%d)", h, n)
	if h > 10 {
		t.Fatalf("tree height %d too tall for %d entries", h, n)
	}
}

func TestBuildTreeCompareWithIncremental(t *testing.T) {
	// Start the testbed for comparing tree construction methods.
	ctx := context.Background()
	tb, oc := buildTestEnv(t)

	// Prepare identical keys and values for both trees.
	n := 50
	keys := make([][]byte, n)
	vals := make([][]byte, n)
	for i := range n {
		keys[i] = fmt.Appendf(nil, "key-%04d", i)
		vals[i] = fmt.Appendf(nil, "val-%04d", i)
	}

	// Build tree incrementally via Set().
	incTree := NewAVLTree(oc)
	itx, err := incTree.NewAVLTreeTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		if err := itx.Set(ctx, keys[i], vals[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := itx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Build tree via BuildTree.
	valRefs := make([]*block.BlockRef, n)
	for i := range n {
		valRefs[i] = writeValue(t, tb.Volume, vals[i])
	}

	// Build the bulk tree from the persisted value references.
	btx, _, err := BuildTree(tb.Volume, nil, nil, sortedEntries(keys, valRefs))
	if err != nil {
		t.Fatal(err)
	}

	// Persist the bulk tree for comparison with the incremental tree.
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the persisted bulk root through the bucket cursor.
	objRef := &bucket.ObjectRef{RootRef: rootRef}
	fc, err := oc.FollowRef(ctx, objRef)
	if err != nil {
		t.Fatal(err)
	}
	bulkTree := NewAVLTree(fc)

	// Compare: both trees should have same keys.
	itx2, err := incTree.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer itx2.Discard()

	// Open the bulk tree for readback alongside the incremental tree.
	btx2, err := bulkTree.NewAVLTreeTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer btx2.Discard()

	// Verify both construction methods preserve the same key membership.
	for i := range n {
		iOk, iErr := itx2.Exists(ctx, keys[i])
		bOk, bErr := btx2.Exists(ctx, keys[i])
		if iErr != nil {
			t.Fatal(iErr)
		}
		if bErr != nil {
			t.Fatal(bErr)
		}
		if iOk != bOk {
			t.Fatalf("key %s: incremental found=%v, bulk found=%v", keys[i], iOk, bOk)
		}
	}

	// Compare iteration order.
	iIter := itx2.Iterate(ctx, nil, true, false)
	bIter := btx2.Iterate(ctx, nil, true, false)
	defer iIter.Close()
	defer bIter.Close()

	// Collect both iteration sequences for comparison.
	var iKeys, bKeys []string
	for iIter.Next() && iIter.Valid() {
		iKeys = append(iKeys, string(iIter.Key()))
	}
	for bIter.Next() && bIter.Valid() {
		bKeys = append(bKeys, string(bIter.Key()))
	}

	// Verify incremental and bulk construction yield the same key order.
	if !slices.Equal(iKeys, bKeys) {
		t.Fatalf("iteration order mismatch:\n  incremental: %v\n  bulk: %v", iKeys, bKeys)
	}
}
