//go:build darwin || linux || windows

package s4db_test

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
)

// TestGetBatch preserves caller order, buffered changes, and independent values.
func TestGetBatch(t *testing.T) {
	// Open the file for inline and packed values.
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "batch.s4wave")
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Write the inline and packed values in one durable transaction.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	packed := bytes.Repeat([]byte("p"), 2048)
	for key, value := range map[string][]byte{"a": []byte("old"), "b": nil, "c": packed, "d": []byte("delete")} {
		if err := tx.Set(ctx, []byte(key), value); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Checkpoint the index before opening a fresh handle.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the checkpointed file for batch reads.
	db, err = s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	// Buffer an update and a deletion before reading an unsorted batch.
	tx, err = db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte("a"), []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Delete(ctx, []byte("d")); err != nil {
		t.Fatal(err)
	}

	// Read mixed keys without changing the caller's order.
	keys := [][]byte{[]byte("c"), []byte("a"), []byte("missing"), []byte("b"), []byte("d"), []byte("c")}
	original := slices.Clone(keys)
	values, found, err := kvtx.GetBatch(ctx, tx, keys)
	if err != nil {
		t.Fatal(err)
	}

	// Require aligned presence, unchanged input, and the snapshot's values.
	if !slices.Equal(found, []bool{true, true, false, true, false, true}) {
		t.Fatalf("presence = %v", found)
	}
	if !slices.EqualFunc(keys, original, bytes.Equal) {
		t.Fatal("batch reordered input keys")
	}
	want := [][]byte{packed, []byte("new"), nil, nil, nil, packed}
	if !slices.EqualFunc(values, want, bytes.Equal) {
		t.Fatal("batch values do not match buffered changes and the index")
	}
	values[0][0] = 'x'
	if !bytes.Equal(values[5], packed) {
		t.Fatal("duplicate results share mutable bytes")
	}

	// Preserve transaction errors and honor cancellation during a batch.
	if _, _, err := kvtx.GetBatch(ctx, tx, [][]byte{nil}); !errors.Is(err, kvtx.ErrEmptyKey) {
		t.Fatalf("empty key error = %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := kvtx.GetBatch(canceled, tx, keys); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled batch error = %v", err)
	}
	tx.Discard()
	if _, _, err := kvtx.GetBatch(ctx, tx, keys); !errors.Is(err, kvtx.ErrDiscarded) {
		t.Fatalf("discarded batch error = %v", err)
	}
}
