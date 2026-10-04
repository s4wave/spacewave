package object_mock

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
)

// TestPrefixer checks that prefixed transactions preserve key access semantics.
func TestPrefixer(t *testing.T) {
	// Build a prefixed view over the test ObjectStore.
	ctx := context.Background()
	objs, _ := BuildTestStore(t)
	pf := object.NewPrefixer(objs, []byte("test-prefix/"))

	// Reuse the prefixed store's transaction constructor in each phase.
	newTx := func(t *testing.T, write bool) kvtx.Tx {
		tx, err := pf.NewTransaction(ctx, write)
		if err != nil {
			t.Fatal(err.Error())
		}
		return tx
	}
	testSeq := "testing123"

	// Write and commit one key through the prefixer.
	tx := newTx(t, true)
	if err := tx.Set(ctx, []byte("test"), []byte(testSeq)); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Read the committed value and check its presence and contents.
	tx = newTx(t, false)
	val, found, err := tx.Get(ctx, []byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.FailNow()
	}
	if string(val) != testSeq {
		t.FailNow()
	}
	tx.Discard()

	// Scan the prefix and assert that its key is visible.
	tx = newTx(t, false)
	var keys []string
	err = tx.ScanPrefix(ctx, nil, func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if keys[0] != "test" {
		t.Fatalf("expected test, got %s", keys[0])
	}
	tx.Discard()

	// Delete the key through a writable prefixed transaction.
	tx = newTx(t, true)
	if err := tx.Delete(ctx, []byte("test")); err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()
}
