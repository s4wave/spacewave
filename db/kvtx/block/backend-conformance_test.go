package kvtx_block

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/kvtx"
)

func TestKVTXBackendConformance(t *testing.T) {
	for _, impl := range backendConformanceImpls() {
		t.Run(impl.String(), func(t *testing.T) {
			t.Run("key-value-commit-discard-reopen", func(t *testing.T) {
				testBackendKeyValueCommitDiscardReopen(t, impl)
			})
			t.Run("sorted-iteration-and-seek", func(t *testing.T) {
				testBackendSortedIterationAndSeek(t, impl)
			})
			t.Run("cursor-and-blob-values", func(t *testing.T) {
				testBackendCursorAndBlobValues(t, impl)
			})
			t.Run("write-delete-commit-churn", func(t *testing.T) {
				testBackendWriteDeleteCommitChurn(t, impl)
			})
		})
	}
}

func TestKVTXBackendGCRefGraphConformance(t *testing.T) {
	ctx := context.Background()
	for _, impl := range backendConformanceImpls() {
		t.Run(impl.String(), func(t *testing.T) {
			tree, refGraph := buildBenchKVTreeWithGC(t, impl, makeBenchKeys(128, benchKeySequential))
			runBenchKVUpdates(t, ctx, tree, 0, 8)
			if refGraph.applyBatches.Load() == 0 {
				t.Fatal("expected GC/refgraph apply batch")
			}
			if refGraph.addRefs.Load() == 0 {
				t.Fatal("expected GC/refgraph added refs")
			}
		})
	}
}

func testBackendKeyValueCommitDiscardReopen(t *testing.T, impl KVImplType) {
	// Create a saved KV root for commit and discard checks.
	ctx := context.Background()
	root := newBackendConformanceRoot(t, ctx, impl)

	// Write and commit the initial KV entries.
	_, btx, tx := root.newWriteTx(t, ctx)
	for _, entry := range []struct {
		key   string
		value string
	}{
		{key: "alpha", value: "one"},
		{key: "bravo", value: "two"},
		{key: "charlie", value: "three"},
	} {
		// Store the KV entry in the write transaction.
		if err := tx.Set(ctx, []byte(entry.key), []byte(entry.value)); err != nil {
			tx.Discard()
			t.Fatal(err)
		}

		// Verify the KV entry exists before commit.
		exists, err := tx.Exists(ctx, []byte(entry.key))
		if err != nil {
			tx.Discard()
			t.Fatal(err)
		}
		if !exists {
			tx.Discard()
			t.Fatalf("%s did not exist after set", entry.key)
		}
	}
	root.commit(t, ctx, btx, tx)

	// Verify the reopened KV values and missing-key behavior.
	readTx := root.newReadTx(t, ctx)
	assertBackendValue(t, ctx, readTx, "alpha", "one")
	assertBackendValue(t, ctx, readTx, "bravo", "two")
	assertBackendValue(t, ctx, readTx, "charlie", "three")
	if exists, err := readTx.Exists(ctx, []byte("missing")); err != nil {
		readTx.Discard()
		t.Fatal(err)
	} else if exists {
		readTx.Discard()
		t.Fatal("missing key exists")
	}

	// Verify the KV reader rejects empty keys.
	if _, _, err := readTx.Get(ctx, nil); err != kvtx.ErrEmptyKey {
		readTx.Discard()
		t.Fatalf("Get(empty) err = %v, want %v", err, kvtx.ErrEmptyKey)
	}
	if _, err := readTx.Exists(ctx, nil); err != kvtx.ErrEmptyKey {
		readTx.Discard()
		t.Fatalf("Exists(empty) err = %v, want %v", err, kvtx.ErrEmptyKey)
	}
	readTx.Discard()

	// Discard a KV deletion and an additional write.
	_, _, tx = root.newWriteTx(t, ctx)
	if err := tx.Delete(ctx, []byte("alpha")); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("discarded"), []byte("discarded")); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	tx.Discard()

	// Verify discarded KV changes leave the saved root intact.
	readTx = root.newReadTx(t, ctx)
	assertBackendValue(t, ctx, readTx, "alpha", "one")
	if _, found, err := readTx.Get(ctx, []byte("discarded")); err != nil {
		readTx.Discard()
		t.Fatal(err)
	} else if found {
		readTx.Discard()
		t.Fatal("discarded write survived")
	}
	readTx.Discard()
}

func testBackendSortedIterationAndSeek(t *testing.T, impl KVImplType) {
	// Create a saved KV root with unsorted keys.
	ctx := context.Background()
	root := newBackendConformanceRoot(t, ctx, impl)
	_, btx, tx := root.newWriteTx(t, ctx)
	for _, key := range []string{"b/20", "a/01", "a/03", "b/10", "a/02", "c/01"} {
		if err := tx.Set(ctx, []byte(key), []byte("value-"+key)); err != nil {
			tx.Discard()
			t.Fatal(err)
		}
	}
	root.commit(t, ctx, btx, tx)

	// Verify KV prefix ordering and seek boundaries.
	readTx := root.newReadTx(t, ctx)
	defer readTx.Discard()
	assertIteratorKeys(t, readTx.Iterate(ctx, []byte("a/"), true, false), nil, []string{"a/01", "a/02", "a/03"})
	assertIteratorKeys(t, readTx.Iterate(ctx, []byte("a/"), true, true), nil, []string{"a/03", "a/02", "a/01"})
	assertIteratorKeys(t, readTx.Iterate(ctx, []byte("a/"), true, false), []byte("a/02"), []string{"a/02", "a/03"})
	assertIteratorKeys(t, readTx.Iterate(ctx, []byte("b/"), true, false), []byte("a/99"), []string{"b/10", "b/20"})
	assertIteratorKeys(t, readTx.Iterate(ctx, []byte("b/"), true, true), []byte("b/15"), []string{"b/10"})
}

func testBackendCursorAndBlobValues(t *testing.T, impl KVImplType) {
	// Create a KV root for cursor and blob values.
	ctx := context.Background()
	root := newBackendConformanceRoot(t, ctx, impl)
	rootCursor, btx, tx := root.newWriteTx(t, ctx)

	// Store an example block through a KV cursor.
	valueCursor := rootCursor.Detach(false)
	valueCursor.ClearAllRefs()
	valueCursor.SetBlock(block_mock.NewExample("cursor-value"), true)
	if err := tx.SetCursorAtKey(ctx, []byte("cursor"), valueCursor, false); err != nil {
		tx.Discard()
		t.Fatal(err)
	}

	// Store and commit a blob through a KV cursor.
	blobData := []byte("blob-value-through-kvtx")
	blobCursor := rootCursor.Detach(false)
	blobCursor.ClearAllRefs()
	if _, err := blob.BuildBlob(ctx, int64(len(blobData)), bytes.NewReader(blobData), blobCursor, nil); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := tx.SetCursorAtKey(ctx, []byte("blob"), blobCursor, true); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	root.commit(t, ctx, btx, tx)

	// Open the saved KV cursor and verify its example block.
	readTx := root.newReadTx(t, ctx)
	cursor, err := readTx.GetCursorAtKey(ctx, []byte("cursor"))
	if err != nil {
		readTx.Discard()
		t.Fatal(err)
	}
	example, err := block_mock.UnmarshalExample(ctx, cursor)
	if err != nil {
		readTx.Discard()
		t.Fatal(err)
	}
	if example.GetMsg() != "cursor-value" {
		readTx.Discard()
		t.Fatalf("cursor value = %q, want cursor-value", example.GetMsg())
	}

	// Verify the saved KV blob bytes.
	value, found, err := readTx.Get(ctx, []byte("blob"))
	if err != nil {
		readTx.Discard()
		t.Fatal(err)
	}
	if !found || !bytes.Equal(value, blobData) {
		readTx.Discard()
		t.Fatalf("blob value = %q, %v, want %q, true", value, found, blobData)
	}
	readTx.Discard()

	// Delete the KV cursor and verify the returned example block.
	_, btx, tx = root.newWriteTx(t, ctx)
	deletedCursor, err := tx.DeleteCursorAtKey(ctx, []byte("cursor"))
	if err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	deletedExample, err := block_mock.UnmarshalExample(ctx, deletedCursor)
	if err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if deletedExample.GetMsg() != "cursor-value" {
		tx.Discard()
		t.Fatalf("deleted cursor value = %q, want cursor-value", deletedExample.GetMsg())
	}
	root.commit(t, ctx, btx, tx)

	// Verify the committed deletion removes the KV cursor.
	readTx = root.newReadTx(t, ctx)
	cursor, err = readTx.GetCursorAtKey(ctx, []byte("cursor"))
	if err != nil {
		readTx.Discard()
		t.Fatal(err)
	}
	if cursor != nil {
		readTx.Discard()
		t.Fatal("deleted cursor key still returned a cursor")
	}
	readTx.Discard()
}

func testBackendWriteDeleteCommitChurn(t *testing.T, impl KVImplType) {
	// Create a saved KV root for repeated write and delete commits.
	ctx := context.Background()
	root := newBackendConformanceRoot(t, ctx, impl)

	// Track expected KV values through successive commits.
	expected := make(map[string]string)
	for step := range 32 {
		// Write the next group of KV values.
		_, btx, tx := root.newWriteTx(t, ctx)
		for i := range 4 {
			key := string(makeSequentialBenchKey(step*4 + i))
			value := string(benchValue(step*4 + i))
			if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
				tx.Discard()
				t.Fatal(err)
			}
			expected[key] = value
		}

		// Remove an older KV key from the transaction and expected values.
		if step >= 8 {
			key := string(makeSequentialBenchKey(step - 8))
			if err := tx.Delete(ctx, []byte(key)); err != nil {
				tx.Discard()
				t.Fatal(err)
			}
			delete(expected, key)
		}
		root.commit(t, ctx, btx, tx)

		// Verify the committed KV size after each step.
		readTx := root.newReadTx(t, ctx)
		size, err := readTx.Size(ctx)
		readTx.Discard()
		if err != nil {
			t.Fatal(err)
		}
		if size != uint64(len(expected)) {
			t.Fatalf("step %d size = %d, want %d", step, size, len(expected))
		}
	}

	// Verify the final KV size and every retained value.
	readTx := root.newReadTx(t, ctx)
	defer readTx.Discard()
	size, err := readTx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != uint64(len(expected)) {
		t.Fatalf("size = %d, want %d", size, len(expected))
	}
	for key, value := range expected {
		got, found, err := readTx.Get(ctx, []byte(key))
		if err != nil {
			t.Fatal(err)
		}
		if !found || !bytes.Equal(got, []byte(value)) {
			t.Fatalf("Get(%x) = %x, %v, want %x, true", key, got, found, value)
		}
	}
}

type backendConformanceRoot struct {
	store   block.StoreOps
	rootRef *block.BlockRef
}

func backendConformanceImpls() []KVImplType {
	return []KVImplType{
		KVImplType_KV_IMPL_TYPE_IAVL,
		KVImplType_KV_IMPL_TYPE_OKRA,
		KVImplType_KV_IMPL_TYPE_OKRA_INLINE,
	}
}

func newBackendConformanceRoot(t *testing.T, ctx context.Context, impl KVImplType) *backendConformanceRoot {
	// Persist an empty KV root in the mock block store.
	t.Helper()
	store := block_mock.NewMockStore(0)
	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(NewKeyValueStore(impl), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return &backendConformanceRoot{store: store, rootRef: rootRef}
}

func (r *backendConformanceRoot) newReadTx(t *testing.T, ctx context.Context) kvtx.BlockTx {
	// Open a KV reader against the saved root.
	t.Helper()
	_, rootCursor := block.NewTransaction(r.store, nil, r.rootRef, nil)
	tx, err := BuildKvTransaction(ctx, rootCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func (r *backendConformanceRoot) newWriteTx(
	t *testing.T,
	ctx context.Context,
) (*block.Cursor, *block.Transaction, kvtx.BlockTx) {
	// Open a KV writer against the saved root.
	t.Helper()
	btx, rootCursor := block.NewTransaction(r.store, nil, r.rootRef, nil)
	tx, err := BuildKvTransaction(ctx, rootCursor, true)
	if err != nil {
		t.Fatal(err)
	}
	return rootCursor, btx, tx
}

func (r *backendConformanceRoot) commit(
	t *testing.T,
	ctx context.Context,
	btx *block.Transaction,
	tx kvtx.BlockTx,
) {
	// Commit the KV changes and advance the saved block root.
	t.Helper()
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	tx.Discard()
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	r.rootRef = rootRef
}

func assertBackendValue(t *testing.T, ctx context.Context, tx kvtx.BlockTx, key, value string) {
	// Verify the KV key returns its expected value.
	t.Helper()
	got, found, err := tx.Get(ctx, []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, []byte(value)) {
		t.Fatalf("Get(%q) = %q, %v, want %q, true", key, got, found, value)
	}
}

func assertIteratorKeys(t *testing.T, it kvtx.Iterator, seek []byte, want []string) {
	// Attribute iterator failures to the calling test and arrange cleanup.
	t.Helper()
	defer it.Close()

	// Verify the KV iterator seek, keys, and terminal state.
	if err := it.Seek(seek); err != nil {
		t.Fatal(err)
	}
	for _, key := range want {
		if !it.Valid() {
			t.Fatalf("iterator ended before %q", key)
		}
		if string(it.Key()) != key {
			t.Fatalf("iterator key = %q, want %q", it.Key(), key)
		}
		it.Next()
	}
	if it.Valid() {
		t.Fatalf("iterator has extra key %q", it.Key())
	}
	if it.Next() || it.Valid() {
		t.Fatalf("iterator restarted after exhaustion at %q", it.Key())
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
}
