package kvtx_block_okra

import (
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
)

func TestWriteBatchWrapperCapabilityAndLifecycle(t *testing.T) {
	ctx := t.Context()
	_, cursor := block.NewTransaction(newOkraTestStore(), nil, nil, nil)
	tree, err := NewTxWithInlineValues(ctx, cursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Discard()
	store := kvtx.NewTxStore(tree)
	prefixed := kvtx_prefixer.NewPrefixer(store, []byte("scope/"))
	for _, s := range []kvtx.Store{store, prefixed} {
		read, err := s.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := read.(kvtx.WriteBatchTxOps); ok {
			t.Fatal("read wrapper advertises batch writes")
		}
		read.Discard()
	}
	// Hide the optional API without replacing the real transaction operations.
	scalar := kvtx.NewTxStore(struct{ kvtx.TxOps }{tree})
	for _, s := range []kvtx.Store{scalar, kvtx_prefixer.NewPrefixer(scalar, []byte("scope/"))} {
		tx, err := s.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := tx.(kvtx.WriteBatchTxOps); ok {
			t.Fatal("scalar wrapper advertises unsupported batches")
		}
		tx.Discard()
	}
	tx, err := prefixed.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	batch, ok := tx.(kvtx.WriteBatchTxOps)
	if !ok {
		t.Fatal("prefixer lost write capability")
	}
	if err := batch.ApplyWriteBatch(ctx, []kvtx.WriteBatchEntry{{Key: []byte("key"), Value: []byte("value")}, {Key: nil}}); !errors.Is(err, kvtx.ErrEmptyKey) {
		t.Fatalf("empty key: %v", err)
	}
	if n, _ := tree.Size(ctx); n != 0 {
		t.Fatal("invalid batch mutated tree")
	}
	input := []kvtx.WriteBatchEntry{{Key: []byte("key"), Value: []byte("value")}}
	if err := batch.ApplyWriteBatch(ctx, input); err != nil {
		t.Fatal(err)
	}
	if string(input[0].Key) != "key" {
		t.Fatal("prefixer mutated input")
	}
	if v, found, err := tree.Get(ctx, []byte("scope/key")); err != nil || !found || string(v) != "value" {
		t.Fatalf("prefix: %q %v %v", v, found, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := batch.ApplyWriteBatch(ctx, input); !errors.Is(err, kvtx.ErrDiscarded) {
		t.Fatalf("write after Commit: %v", err)
	}
	// Committing the virtual wrapper must not finalize the enclosing Okra tx.
	if err := tree.Set(ctx, []byte("still-open"), nil); err != nil {
		t.Fatal(err)
	}
	direct, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	direct.Discard()
	if err := direct.(kvtx.WriteBatchTxOps).ApplyWriteBatch(ctx, input); !errors.Is(err, kvtx.ErrDiscarded) {
		t.Fatalf("write after Discard: %v", err)
	}
}
