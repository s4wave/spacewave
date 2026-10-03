package kvtx_block_okra

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/kvtx"
)

func TestWriteBatchMatchesScalarRoots(t *testing.T) {
	for _, inline := range []bool{false, true} {
		t.Run(fmt.Sprintf("inline=%v", inline), func(t *testing.T) {
			// Create the shared store and transaction constructor for this value layout.
			ctx := t.Context()
			store := newOkraTestStore()
			open := func() (*Tx, *block.Transaction) {
				// Open an Okra tree with the selected inline value layout.
				btx, cursor := block.NewTransaction(store, nil, nil, nil)
				tx, err := NewTx(ctx, cursor, nil, true, nil)
				if err != nil {
					t.Fatal(err)
				}
				tx.inlineValues = inline
				return tx, btx
			}

			// Open independent batch and scalar trees and track their expected values.
			batch, batchBlock := open()
			scalar, scalarBlock := open()
			defer batch.Discard()
			defer scalar.Discard()
			rng := rand.New(rand.NewPCG(15, 27))
			values := make(map[string][]byte)

			// Churn across leaf boundaries, including repeated keys, empty values,
			// large external blobs, missing deletes and replacement boundaries.
			for round := range 32 {
				// Generate this round of changes and apply them to the scalar tree.
				entries := make([]kvtx.WriteBatchEntry, 97)
				for i := range entries {
					key := fmt.Sprintf("key-%04d", rng.IntN(1024))
					entry := kvtx.WriteBatchEntry{Key: []byte(key), Delete: rng.IntN(4) == 0}
					if !entry.Delete {
						entry.Value = bytes.Repeat([]byte{byte(round), byte(i)}, rng.IntN(180))
						if err := scalar.Set(ctx, entry.Key, entry.Value); err != nil {
							t.Fatal(err)
						}
						values[key] = bytes.Clone(entry.Value)
					} else {
						if err := scalar.Delete(ctx, entry.Key); err != nil {
							t.Fatal(err)
						}
						delete(values, key)
					}
					entries[i] = entry
				}

				// Apply the same changes as one batch.
				if err := batch.ApplyWriteBatch(ctx, entries); err != nil {
					t.Fatal(err)
				}

				// Require the batch and scalar roots to agree after this round.
				if batch.root.GetSize() != scalar.root.GetSize() || batch.root.GetHeight() != scalar.root.GetHeight() || !bytes.Equal(batch.root.GetRootHash(), scalar.root.GetRootHash()) {
					t.Fatalf("round %d: batch root differs from scalar root", round)
				}

				// Overwrite the caller buffers after return; published bytes must
				// belong to the tree, including its keys and inline values.
				for _, entry := range entries {
					clear(entry.Key)
					clear(entry.Value)
				}

				// Require batch values to retain their bytes after caller buffers are cleared.
				for key, want := range values {
					got, found, err := batch.Get(ctx, []byte(key))
					if err != nil || !found || !bytes.Equal(got, want) {
						t.Fatalf("round %d key %s: %v", round, key, err)
					}
				}
			}

			// Publish the batch and scalar trees for durable root comparison.
			batchRef, _, err := batchBlock.Write(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			scalarRef, _, err := scalarBlock.Write(ctx, true)
			if err != nil {
				t.Fatal(err)
			}

			// Require the durable roots to match.
			if !batchRef.EqualsRef(scalarRef) {
				t.Fatal("materialized roots differ")
			}

			// Require all expected values to read back from the published batch root.
			read := openOkraRoot(t, ctx, store, batchRef, false)
			defer read.Discard()
			for key, want := range values {
				got, found, err := read.Get(ctx, []byte(key))
				if err != nil || !found || !bytes.Equal(got, want) {
					t.Fatalf("readback %s: %v", key, err)
				}
			}
		})
	}
}

func TestWriteBatchPreservesSnapshotsAndSparsePages(t *testing.T) {
	// Open a writable tree using inline values.
	ctx := t.Context()
	store := newOkraTestStore()
	btx, cursor := block.NewTransaction(store, nil, nil, nil)
	tx, err := NewTxWithInlineValues(ctx, cursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Build and publish a dense fixture through one write batch.
	const count = 2048
	entries := make([]kvtx.WriteBatchEntry, count)
	for i := range entries {
		key := []byte(fmt.Sprintf("key-%04d", i))
		entries[i] = kvtx.WriteBatchEntry{Key: key, Value: key}
	}
	if err := tx.ApplyWriteBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if _, _, err := btx.Write(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Retain the reference of a middle page outside the sparse changes.
	middle := []byte("key-1024")
	path, err := tx.findPagePath(ctx, 0, okraNodeKey{key: middle})
	if err != nil {
		t.Fatal(err)
	}
	middleRef := path.leaf().cursor.GetRef().Clone()

	// Retain a snapshot iterator and the first value cursor.
	iter := tx.BlockIterate(ctx, nil, true, false)
	defer iter.Close()
	if !iter.Next() {
		t.Fatal(iter.Err())
	}
	saved := iter.ValueCursor()

	// Replace the first value and delete the last key in one batch.
	if err := tx.ApplyWriteBatch(ctx, []kvtx.WriteBatchEntry{
		{Key: []byte("key-0000"), Value: []byte("replacement")},
		{Key: []byte("key-2047"), Delete: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Require the sparse changes to preserve the untouched middle page.
	path, err = tx.findPagePath(ctx, 0, okraNodeKey{key: middle})
	if err != nil {
		t.Fatal(err)
	}
	if !path.leaf().cursor.GetRef().EqualsRef(middleRef) || path.leaf().cursor.IsDirty() {
		t.Fatal("sparse mutation rebuilt an untouched middle page")
	}

	// A write through the new tree must not alter the retained inline value.
	if _, _, err := btx.Write(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Require the retained snapshot to expose every original value.
	for i := range count {
		value, err := iter.Value()
		if !iter.Valid() || err != nil || string(value) != fmt.Sprintf("key-%04d", i) {
			t.Fatalf("snapshot %d: %q, %v", i, value, err)
		}
		iter.Next()
	}

	// Require snapshot traversal to finish without an iterator error.
	if iter.Valid() || iter.Err() != nil {
		t.Fatal("snapshot completion", iter.Err())
	}

	// Require the saved value cursor to survive closing the snapshot.
	iter.Close()
	if got, err := blob.FetchToBytes(ctx, saved); err != nil || string(got) != "key-0000" {
		t.Fatalf("retained inline cursor = %q, %v", got, err)
	}

	// Delete the entire tree, including every boundary, then start again.
	for i := range entries {
		entries[i].Delete = true
	}
	if err := tx.ApplyWriteBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if tx.root.GetSize() != 0 {
		t.Fatal("expected empty root")
	}

	// Insert a new empty value after deleting the whole tree.
	if err := tx.ApplyWriteBatch(ctx, []kvtx.WriteBatchEntry{{Key: []byte("new")}}); err != nil {
		t.Fatal(err)
	}

	// Require the new key to store an empty value.
	if got, found, err := tx.Get(ctx, []byte("new")); err != nil || !found || len(got) != 0 {
		t.Fatal("nil must store empty value", got, found, err)
	}
}

func TestWriteBatchValidationAndLifecycle(t *testing.T) {
	// Open a writable tree for batch validation and lifecycle checks.
	ctx := t.Context()
	_, root := block.NewTransaction(newOkraTestStore(), nil, nil, nil)
	tx, err := NewTxWithInlineValues(ctx, root, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Require an invalid batch to leave the tree unchanged.
	if err := tx.ApplyWriteBatch(ctx, []kvtx.WriteBatchEntry{{Key: []byte("valid")}, {}}); !errors.Is(err, kvtx.ErrEmptyKey) {
		t.Fatal(err)
	}
	if tx.root.GetSize() != 0 {
		t.Fatal("invalid batch mutated tree")
	}

	// Require batch writes to respect a canceled context.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := tx.ApplyWriteBatch(canceled, []kvtx.WriteBatchEntry{{Key: []byte("a")}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	// Require a read-only tree to reject batch writes.
	tx.write = false
	if err := tx.ApplyWriteBatch(ctx, nil); !errors.Is(err, kvtx.ErrNotWrite) {
		t.Fatal(err)
	}

	// Require a discarded tree to reject further batch writes.
	tx.write = true
	tx.Discard()
	if err := tx.ApplyWriteBatch(ctx, nil); !errors.Is(err, kvtx.ErrDiscarded) {
		t.Fatal(err)
	}
}

func BenchmarkOkraWriteBatch(b *testing.B) {
	for _, batch := range []bool{false, true} {
		b.Run(fmt.Sprintf("batch=%v", batch), func(b *testing.B) {
			// Open a writable inline-value tree for this benchmark mode.
			ctx := b.Context()
			_, root := block.NewTransaction(newOkraTestStore(), nil, nil, nil)
			tx, err := NewTxWithInlineValues(ctx, root, nil, true, nil)
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Discard()

			// Populate the benchmark tree before measuring updates.
			entries := make([]kvtx.WriteBatchEntry, 512)
			for i := range entries {
				entries[i] = kvtx.WriteBatchEntry{Key: []byte(fmt.Sprintf("key-%04d", i)), Value: []byte("value")}
			}
			if err := tx.ApplyWriteBatch(ctx, entries); err != nil {
				b.Fatal(err)
			}

			// Measure repeated updates through the selected batch or scalar API.
			b.ReportAllocs()
			b.ResetTimer()
			for n := range b.N {
				// Prepare the next group of benchmark value replacements.
				writes := make([]kvtx.WriteBatchEntry, 32)
				for i := range writes {
					writes[i] = kvtx.WriteBatchEntry{Key: entries[(n*32+i)%len(entries)].Key, Value: []byte(fmt.Sprintf("value-%d", n))}
				}

				// Apply the prepared replacements through the selected write API.
				if batch {
					if err := tx.ApplyWriteBatch(ctx, writes); err != nil {
						b.Fatal(err)
					}
				} else {
					for _, entry := range writes {
						if err := tx.Set(ctx, entry.Key, entry.Value); err != nil {
							b.Fatal(err)
						}
					}
				}
			}
		})
	}
}
