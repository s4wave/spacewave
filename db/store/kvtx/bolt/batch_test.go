//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"os"
	"path"
	"testing"
)

// newBatchTestStore opens a bolt store with a batch wrapper for tests.
func newBatchTestStore(t *testing.T, batchSize int) *BatchStore {
	// Allocate a temporary directory for the batched Bolt test store.
	t.Helper()
	dir, err := os.MkdirTemp("", "hydra-test-batch-")
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Open the Bolt test store and register its database cleanup.
	db, err := Open(path.Join(dir, "database.boltdb"), 0o644, nil, []byte("test-bucket"))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { _ = db.db.Close() })
	return NewBatchStore(db, batchSize)
}

// TestBatchStoreAbandonedTxDoesNotWedgeStore tests that abandoning a
// batched write transaction without Commit or Discard does not block the
// store: a later transaction must open and flush normally.
func TestBatchStoreAbandonedTxDoesNotWedgeStore(t *testing.T) {
	// Start a BatchStore for the abandoned transaction test.
	ctx := context.Background()
	b := newBatchTestStore(t, 4)

	// Abandon one transaction entirely.
	tx, err := b.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Set(ctx, []byte("lost"), []byte("value")); err != nil {
		t.Fatal(err.Error())
	}
	// No Commit, no Discard.

	// The store must remain usable.
	tx2, err := b.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("store wedged after abandoned tx: %v", err)
	}
	if err := tx2.Set(ctx, []byte("kept"), []byte("value")); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err.Error())
	}

	// Verify a later committed key remains readable after an abandoned transaction.
	rtx, err := b.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	val, found, err := rtx.Get(ctx, []byte("kept"))
	if err != nil || !found || string(val) != "value" {
		t.Fatalf("expected kept=value, found=%v err=%v", found, err)
	}
	rtx.Discard()
}

// TestBatchStoreDiscardDoesNotEraseCommittedWrites tests that discarding a
// virtual transaction before a flush leaves other committed transactions'
// writes intact.
func TestBatchStoreDiscardDoesNotEraseCommittedWrites(t *testing.T) {
	// Start a BatchStore for the discarded transaction test.
	ctx := context.Background()
	b := newBatchTestStore(t, 8)

	// Commit a buffered key before discarding another transaction.
	tx1, err := b.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := tx1.Set(ctx, []byte("one"), []byte("1")); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Discard a buffered insertion without changing the committed batch.
	tx3, err := b.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := tx3.Set(ctx, []byte("three"), []byte("3")); err != nil {
		t.Fatal(err.Error())
	}
	tx3.Discard()

	// Flush and verify both committed keys survive and the discarded key
	// is absent.
	if err := b.Flush(); err != nil {
		t.Fatal(err.Error())
	}
	rtx, err := b.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	for key, want := range map[string]string{"one": "1"} {
		val, found, err := rtx.Get(ctx, []byte(key))
		if err != nil || !found || string(val) != want {
			t.Fatalf("expected %s=%s after discard+flush, found=%v val=%q err=%v", key, want, found, val, err)
		}
	}
	_, found3, err := rtx.Get(ctx, []byte("three"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if found3 {
		t.Fatal("discarded key three should not exist")
	}
	rtx.Discard()
}

// TestBatchStoreReadYourWrites tests that a transaction observes its own
// buffered writes before commit.
func TestBatchStoreReadYourWrites(t *testing.T) {
	// Start a BatchStore for buffered read and iteration checks.
	ctx := context.Background()
	b := newBatchTestStore(t, 8)

	// Buffer a key insertion and deletion within one write transaction.
	tx, err := b.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Delete(ctx, []byte("gone")); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the transaction reads its buffered insertion and deletion.
	val, found, err := tx.Get(ctx, []byte("a"))
	if err != nil || !found || string(val) != "1" {
		t.Fatalf("read-your-writes failed for a: found=%v val=%q err=%v", found, val, err)
	}
	_, found, err = tx.Get(ctx, []byte("gone"))
	if err != nil || found {
		t.Fatalf("expected deleted key to be invisible, found=%v err=%v", found, err)
	}

	// Iterate sees the merged view.
	var seen []string
	it := tx.Iterate(ctx, nil, true, false)
	for it.Next() {
		seen = append(seen, string(it.Key()))
	}
	if len(seen) != 1 || seen[0] != "a" {
		t.Fatalf("unexpected iteration view: %v", seen)
	}

	// Commit the transaction after checking its buffered view.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}
