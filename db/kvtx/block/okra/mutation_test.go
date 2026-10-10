package kvtx_block_okra

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
)

func TestTxSetDeleteCommit(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Publish a tree after inserting keys and deleting present and absent keys.
	rootRef := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		// Insert the three keys before exercising deletion.
		if err := tx.Set(ctx, []byte("b"), []byte("2")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("c"), []byte("3")); err != nil {
			t.Fatal(err)
		}

		// Remove the existing key and accept deletion of a missing key.
		if err := tx.Delete(ctx, []byte("b")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Delete(ctx, []byte("missing")); err != nil {
			t.Fatal(err)
		}
	})

	// Open the published Okra tree for readback.
	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()

	// Read the number of keys retained after publication.
	size, err := readTx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Require the published tree to retain both surviving values.
	if size != 2 {
		t.Fatalf("size = %d, want 2", size)
	}
	assertOkraValue(t, ctx, readTx, "a", "1")
	assertOkraValue(t, ctx, readTx, "c", "3")

	// Verify that the deleted key is absent from the published tree.
	exists, err := readTx.Exists(ctx, []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("deleted key still exists")
	}
}

func TestTxSequentialCommitChurnPreservesSize(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Track expected keys across successive Okra commits.
	var rootRef *block.BlockRef
	expected := make(map[string][]byte)
	for step := range 32 {
		// Open the next writable tree from the previous root.
		btx, rootCursor := block.NewTransaction(store, nil, rootRef, nil)
		tx, err := NewTx(ctx, rootCursor, nil, true, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Insert the next group of keys into the expected and actual trees.
		for i := range 4 {
			key := sequentialOkraTestKey(step*4 + i)
			value := []byte{byte(step), byte(i)}
			if err := tx.Set(ctx, key, value); err != nil {
				t.Fatal(err)
			}
			expected[string(key)] = value
		}

		// Remove an older key after the insertion history is long enough.
		if step >= 8 {
			key := sequentialOkraTestKey(step - 8)
			if err := tx.Delete(ctx, key); err != nil {
				t.Fatal(err)
			}
			delete(expected, string(key))
		}

		// Publish this step before checking its durable size.
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		rootRef, _, err = btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}

		// Require the published size to match the expected key set.
		readTx := openOkraRoot(t, ctx, store, rootRef, false)
		size, err := readTx.Size(ctx)
		readTx.Discard()
		if err != nil {
			t.Fatal(err)
		}
		if size != uint64(len(expected)) {
			t.Fatalf("step %d size = %d, want %d", step, size, len(expected))
		}
	}
}

func TestTxSetCursorAtKeyRawValue(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Open a writable Okra tree on the block transaction.
	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Store the raw value block for cursor insertion.
	rawRef, _, err := store.PutBlock(ctx, []byte("raw-value"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Attach the raw block cursor to the requested Okra key.
	valueCursor := rootCursor.Detach(false)
	valueCursor.ClearAllRefs()
	valueCursor.SetRefAtCursor(rawRef, true)
	if err := tx.SetCursorAtKey(ctx, []byte("raw"), valueCursor, false); err != nil {
		t.Fatal(err)
	}

	// Commit the Okra changes and publish the block transaction root.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the published Okra tree for readback.
	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()

	// Read the raw value through the published Okra key.
	value, found, err := readTx.Get(ctx, []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}

	// Require the raw value bytes to survive publication.
	if !found || !bytes.Equal(value, []byte("raw-value")) {
		t.Fatalf("raw value = %q, %v", value, found)
	}
}

func TestTxSetCursorAtKeyDirtyBlockValue(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Open a writable Okra tree on the block transaction.
	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a dirty example block through its value cursor.
	valueCursor := rootCursor.Detach(false)
	valueCursor.ClearAllRefs()
	valueCursor.SetBlock(block_mock.NewExample("dirty block"), true)
	if err := tx.SetCursorAtKey(ctx, []byte("block"), valueCursor, false); err != nil {
		t.Fatal(err)
	}

	// Commit the Okra changes and publish the block transaction root.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the published Okra tree for readback.
	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()

	// Follow and decode the published example block.
	readCursor, err := readTx.GetCursorAtKey(ctx, []byte("block"))
	if err != nil {
		t.Fatal(err)
	}
	example, err := block_mock.UnmarshalExample(ctx, readCursor)
	if err != nil {
		t.Fatal(err)
	}

	// Require the dirty block message to survive publication.
	if example.GetMsg() != "dirty block" {
		t.Fatalf("value block msg = %q, want dirty block", example.GetMsg())
	}
}

func TestTxSetMaterializesBlobOutsideTreeTransaction(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Open a writable Okra tree on the block transaction.
	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a blob value into the writable tree.
	if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}

	// Require the outer transaction to retain only root and page nodes.
	if nodes := len(btx.GetBlockGraph().Nodes()); nodes != 3 {
		t.Fatalf("outer transaction nodes = %d, want 3 root/page nodes", nodes)
	}

	// Commit the Okra changes and publish the block transaction root.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the published Okra tree for readback.
	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()

	// Require the published blob value to read back unchanged.
	assertOkraValue(t, ctx, readTx, "a", "1")
}

func TestNewTxAcceptsDirtyRootWithPendingPageRef(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Open a writable tree whose root page reference is still pending.
	_, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a key while leaving the root unpublished.
	if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}

	// Reopen the dirty root with its pending page reference.
	reopened, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Discard()

	// Require the reopened dirty root to expose the inserted value.
	assertOkraValue(t, ctx, reopened, "a", "1")
}

func TestTxDeleteCursorAtKey(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Publish a tree containing the value to remove.
	rootRef := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
			t.Fatal(err)
		}
	})

	// Open a writable Okra tree on the block transaction.
	btx, rootCursor := block.NewTransaction(store, nil, rootRef, nil)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the key while retaining its value cursor.
	valueCursor, err := tx.DeleteCursorAtKey(ctx, []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if valueCursor == nil {
		t.Fatal("missing deleted value cursor")
	}

	// Read the deleted value through the retained cursor.
	data, err := blob.FetchToBytes(ctx, valueCursor)
	if err != nil {
		t.Fatal(err)
	}

	// Require the retained cursor to preserve the removed value.
	if !bytes.Equal(data, []byte("1")) {
		t.Fatalf("deleted cursor value = %q, want 1", data)
	}

	// Commit the Okra changes and publish the block transaction root.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	nextRoot, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Reopen the tree published after deleting its only key.
	readTx := openOkraRoot(t, ctx, store, nextRoot, false)
	defer readTx.Discard()

	// Require the published tree to contain no keys.
	size, err := readTx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("size after delete = %d, want 0", size)
	}
}

func TestTxDeterministicRootForReorderedWrites(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Publish the values in alphabetical key order.
	first := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for _, key := range []string{"a", "b", "c"} {
			if err := tx.Set(ctx, []byte(key), []byte("value-"+key)); err != nil {
				t.Fatal(err)
			}
		}
	})

	// Publish the same values in a different key order.
	second := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for _, key := range []string{"c", "a", "b"} {
			if err := tx.Set(ctx, []byte(key), []byte("value-"+key)); err != nil {
				t.Fatal(err)
			}
		}
	})

	// Require identical roots regardless of key insertion order.
	if !first.EqualsRef(second) {
		t.Fatalf("root ref changed with write order: %s != %s", first.MarshalLog(), second.MarshalLog())
	}
}

func TestTxCacheDiscardLeavesRootUnchanged(t *testing.T) {
	// Create the context and block store for the Okra mutation test.
	ctx := context.Background()
	store := newOkraTestStore()

	// Open the Okra transaction wrapped by the transaction cache.
	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	okraTx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Open a cached writable transaction on the Okra tree.
	txStore := kvtx_txcache.NewTxStore(okraTx, true)
	discardTx, err := txStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Stage a cached value and discard its transaction.
	if err := discardTx.Set(ctx, []byte("discarded"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	discardTx.Discard()

	// Publish the enclosing tree after discarding the cached transaction.
	if err := okraTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the published Okra tree for readback.
	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()

	// Require the discarded cached key to remain absent.
	exists, err := readTx.Exists(ctx, []byte("discarded"))
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("discarded key exists")
	}
}

func writeMutatedOkraRoot(
	t *testing.T,
	ctx context.Context,
	store block.StoreOps,
	mutate func(*Tx),
) *block.BlockRef {
	// Open an empty writable Okra tree for the supplied mutation.
	t.Helper()
	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Apply the mutation and publish its durable root.
	mutate(tx)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return rootRef
}

func openOkraRoot(
	t *testing.T,
	ctx context.Context,
	store block.StoreOps,
	rootRef *block.BlockRef,
	write bool,
) *Tx {
	// Open the supplied root with the requested Okra transaction access.
	t.Helper()
	_, rootCursor := block.NewTransaction(store, nil, rootRef, nil)
	tx, err := NewTx(ctx, rootCursor, nil, write, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func assertOkraValue(t *testing.T, ctx context.Context, tx *Tx, key, expected string) {
	// Read the requested Okra key for comparison with its expected value.
	t.Helper()
	value, found, err := tx.Get(ctx, []byte(key))
	if err != nil {
		t.Fatal(err)
	}

	// Require the requested key and its expected value bytes.
	if !found || !bytes.Equal(value, []byte(expected)) {
		t.Fatalf("Get(%q) = %q, %v, want %q, true", key, value, found, expected)
	}
}

func sequentialOkraTestKey(i int) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(i))
	return key
}

type stagingCountStore struct {
	block.StoreOps
	mu         sync.Mutex
	putCalls   int
	batchCalls int
	batchSizes []int
}

func (s *stagingCountStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.mu.Lock()
	s.putCalls++
	s.mu.Unlock()
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *stagingCountStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Record the batch publication and its size under the store lock.
	s.mu.Lock()
	s.batchCalls++
	s.batchSizes = append(s.batchSizes, len(entries))
	s.mu.Unlock()

	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *stagingCountStore) counts() (int, int, []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.putCalls, s.batchCalls, append([]int(nil), s.batchSizes...)
}

func TestTxSetStagesValueWritesUntilCommit(t *testing.T) {
	// Open a writable tree on a store that counts publication calls.
	ctx := context.Background()
	store := &stagingCountStore{StoreOps: newOkraTestStore()}
	_, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Insert values and confirm they can be read from staging.
	for i := range 8 {
		// Stage the next value under its Okra key.
		key := []byte{byte('a' + i)}
		value := bytes.Repeat([]byte{byte('1' + i)}, 64)
		if err := tx.Set(ctx, key, value); err != nil {
			t.Fatal(err)
		}

		// Require the staged value to read back before publication.
		got, found, err := tx.Get(ctx, key)
		if err != nil || !found || !bytes.Equal(got, value) {
			t.Fatalf("read staged value %q: found=%v err=%v", key, found, err)
		}
	}

	// Require all value writes to remain staged before commit.
	if puts, batches, _ := store.counts(); puts != 0 || batches != 0 {
		t.Fatalf("value writes reached inner store before commit: puts=%d batches=%d", puts, batches)
	}

	// Commit the tree to publish the staged values and pages.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Require a single batch containing the values and their pages.
	puts, batches, sizes := store.counts()
	if puts != 0 {
		t.Fatalf("expected batched publication, got %d direct puts", puts)
	}
	if batches != 1 {
		t.Fatalf("expected staged values and pages in one batch, got %d (%v)", batches, sizes)
	}
	if sizes[0] <= 8 {
		t.Fatalf("expected 8 staged values and the pages in the batch, got %v", sizes)
	}
}

func TestTxDiscardPublishesNoStagedValues(t *testing.T) {
	// Open a writable tree on a store that counts publication calls.
	ctx := context.Background()
	store := &stagingCountStore{StoreOps: newOkraTestStore()}
	_, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Stage a value and discard the tree transaction.
	if err := tx.Set(ctx, []byte("discarded"), bytes.Repeat([]byte("x"), 64)); err != nil {
		t.Fatal(err)
	}
	tx.Discard()

	// Require discard to leave the underlying store untouched.
	if puts, batches, sizes := store.counts(); puts != 0 || batches != 0 {
		t.Fatalf("discard published staged values: puts=%d batches=%d sizes=%v", puts, batches, sizes)
	}
}
