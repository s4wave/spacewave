package kvtx_block_okra

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
)

func TestTxSetDeleteCommit(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()

	rootRef := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		if err := tx.Set(ctx, []byte("b"), []byte("2")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("c"), []byte("3")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Delete(ctx, []byte("b")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Delete(ctx, []byte("missing")); err != nil {
			t.Fatal(err)
		}
	})

	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()
	size, err := readTx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 2 {
		t.Fatalf("size = %d, want 2", size)
	}
	assertOkraValue(t, ctx, readTx, "a", "1")
	assertOkraValue(t, ctx, readTx, "c", "3")
	exists, err := readTx.Exists(ctx, []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("deleted key still exists")
	}
}

func TestTxSequentialCommitChurnPreservesSize(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()

	var rootRef *block.BlockRef
	expected := make(map[string][]byte)
	for step := range 32 {
		btx, rootCursor := block.NewTransaction(store, nil, rootRef, nil)
		tx, err := NewTx(ctx, rootCursor, nil, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i := range 4 {
			key := sequentialOkraTestKey(step*4 + i)
			value := []byte{byte(step), byte(i)}
			if err := tx.Set(ctx, key, value); err != nil {
				t.Fatal(err)
			}
			expected[string(key)] = value
		}
		if step >= 8 {
			key := sequentialOkraTestKey(step - 8)
			if err := tx.Delete(ctx, key); err != nil {
				t.Fatal(err)
			}
			delete(expected, string(key))
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		rootRef, _, err = btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}

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
	ctx := context.Background()
	store := newOkraTestStore()

	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	rawRef, _, err := store.PutBlock(ctx, []byte("raw-value"), nil)
	if err != nil {
		t.Fatal(err)
	}
	valueCursor := rootCursor.Detach(false)
	valueCursor.ClearAllRefs()
	valueCursor.SetRefAtCursor(rawRef, true)
	if err := tx.SetCursorAtKey(ctx, []byte("raw"), valueCursor, false); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()
	value, found, err := readTx.Get(ctx, []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(value, []byte("raw-value")) {
		t.Fatalf("raw value = %q, %v", value, found)
	}
}

func TestTxSetCursorAtKeyDirtyBlockValue(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()

	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	valueCursor := rootCursor.Detach(false)
	valueCursor.ClearAllRefs()
	valueCursor.SetBlock(block_mock.NewExample("dirty block"), true)
	if err := tx.SetCursorAtKey(ctx, []byte("block"), valueCursor, false); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()
	readCursor, err := readTx.GetCursorAtKey(ctx, []byte("block"))
	if err != nil {
		t.Fatal(err)
	}
	example, err := block_mock.UnmarshalExample(ctx, readCursor)
	if err != nil {
		t.Fatal(err)
	}
	if example.GetMsg() != "dirty block" {
		t.Fatalf("value block msg = %q, want dirty block", example.GetMsg())
	}
}

func TestTxSetMaterializesBlobOutsideTreeTransaction(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()

	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if nodes := len(btx.GetBlockGraph().Nodes()); nodes != 3 {
		t.Fatalf("outer transaction nodes = %d, want 3 root/page nodes", nodes)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()
	assertOkraValue(t, ctx, readTx, "a", "1")
}

func TestNewTxAcceptsDirtyRootWithPendingPageRef(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()

	_, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Discard()
	assertOkraValue(t, ctx, reopened, "a", "1")
}

func TestTxDeleteCursorAtKey(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()
	rootRef := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
			t.Fatal(err)
		}
	})

	btx, rootCursor := block.NewTransaction(store, nil, rootRef, nil)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	valueCursor, err := tx.DeleteCursorAtKey(ctx, []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if valueCursor == nil {
		t.Fatal("missing deleted value cursor")
	}
	data, err := blob.FetchToBytes(ctx, valueCursor)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte("1")) {
		t.Fatalf("deleted cursor value = %q, want 1", data)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	nextRoot, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	readTx := openOkraRoot(t, ctx, store, nextRoot, false)
	defer readTx.Discard()
	size, err := readTx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("size after delete = %d, want 0", size)
	}
}

func TestTxDeterministicRootForReorderedWrites(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()
	first := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for _, key := range []string{"a", "b", "c"} {
			if err := tx.Set(ctx, []byte(key), []byte("value-"+key)); err != nil {
				t.Fatal(err)
			}
		}
	})
	second := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for _, key := range []string{"c", "a", "b"} {
			if err := tx.Set(ctx, []byte(key), []byte("value-"+key)); err != nil {
				t.Fatal(err)
			}
		}
	})
	if !first.EqualsRef(second) {
		t.Fatalf("root ref changed with write order: %s != %s", first.MarshalLog(), second.MarshalLog())
	}
}

func TestTxCacheDiscardLeavesRootUnchanged(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()

	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	okraTx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	txStore := kvtx_txcache.NewTxStore(okraTx, true)
	discardTx, err := txStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := discardTx.Set(ctx, []byte("discarded"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	discardTx.Discard()
	if err := okraTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()
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
	t.Helper()
	btx, rootCursor := block.NewTransaction(store, nil, nil, nil)
	rootCursor.SetBlock(&Root{}, true)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
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
	t.Helper()
	_, rootCursor := block.NewTransaction(store, nil, rootRef, nil)
	tx, err := NewTx(ctx, rootCursor, nil, write, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func assertOkraValue(t *testing.T, ctx context.Context, tx *Tx, key, expected string) {
	t.Helper()
	value, found, err := tx.Get(ctx, []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(value, []byte(expected)) {
		t.Fatalf("Get(%q) = %q, %v, want %q, true", key, value, found, expected)
	}
}

func sequentialOkraTestKey(i int) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(i))
	return key
}
