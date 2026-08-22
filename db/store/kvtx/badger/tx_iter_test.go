package store_kvtx_badger

import (
	"context"
	"testing"

	bdb "github.com/dgraph-io/badger/v4"
)

// TestIterateReleaseRemovesOwnEntry tests that closing one iterator
// removes only that iterator from the transaction's open set.
func TestIterateReleaseRemovesOwnEntry(t *testing.T) {
	o := bdb.DefaultOptions("").WithInMemory(true)
	db, err := Open(o)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.db.Close()

	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()

	it1 := tx.Iterate(ctx, nil, true, false)
	it2 := tx.Iterate(ctx, nil, true, false)

	wt := tx.(*Tx)
	wt.mtx.Lock()
	n := len(wt.iters)
	wt.mtx.Unlock()
	if n != 2 {
		t.Fatalf("expected 2 open iterators, got %d", n)
	}

	it1.Close()
	wt.mtx.Lock()
	n = len(wt.iters)
	wt.mtx.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 open iterator after closing the first, got %d", n)
	}

	it2.Close()
}
