package kvtx_block_okra

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

func TestReplaceAllMatchesIncrementalTreeAndPreservesReaders(t *testing.T) {
	// Create a sorted stream of values for tree replacement.
	ctx := t.Context()
	store := newOkraTestStore()
	values := func(yield func([]byte, []byte) bool) {
		for i := range 1024 {
			key := []byte(fmt.Sprintf("key-%04d", i))
			if !yield(key, key) {
				return
			}
		}
	}

	// Build the expected root through incremental insertion.
	want := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for key, value := range values {
			if err := tx.Set(ctx, key, value); err != nil {
				t.Fatal(err)
			}
		}
	})

	// Replace an existing tree while retaining its snapshot reader.
	got := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		// Store the original value before retaining its reader.
		if err := tx.Set(ctx, []byte("old"), []byte("retained")); err != nil {
			t.Fatal(err)
		}

		// Retain an iterator positioned at the original value.
		reader := tx.Iterate(ctx, nil, true, false)
		defer reader.Close()
		if !reader.Next() {
			t.Fatal("missing old key", reader.Err())
		}

		// Replace the tree with the complete sorted value stream.
		if err := tx.ReplaceAll(ctx, values); err != nil {
			t.Fatal(err)
		}

		// Require the retained snapshot to expose the original value.
		value, err := reader.Value()
		if err != nil || string(value) != "retained" {
			t.Fatalf("old snapshot: %q, %v", value, err)
		}
	})

	// Require bulk replacement to match the incremental root.
	if !want.EqualsRef(got) {
		t.Fatal("bulk replacement differs from incremental tree")
	}

	// Require all streamed values to read back from the replaced tree.
	read := openOkraRoot(t, ctx, store, got, false)
	defer read.Discard()
	for key, want := range values {
		got, found, err := read.Get(ctx, key)
		if err != nil || !found || !bytes.Equal(got, want) {
			t.Fatalf("%s: %q, %v, %v", key, got, found, err)
		}
	}
}

// TestReplaceAllBlocksMatchesIncrementalTree verifies that streamed value
// blocks build the same tree as value cursors set one key at a time, and that
// a value reads back from the written root.
func TestReplaceAllBlocksMatchesIncrementalTree(t *testing.T) {
	// Create a sorted stream of example blocks for tree replacement.
	ctx := t.Context()
	store := newOkraTestStore()
	values := func(yield func([]byte, block.Block) bool) {
		for i := range 512 {
			key := fmt.Sprintf("key-%04d", i)
			if !yield([]byte(key), block_mock.NewExample(key)) {
				return
			}
		}
	}

	// Build the expected root by inserting each block cursor.
	want := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for key, value := range values {
			cursor := tx.bcs.Detach(false)
			cursor.ClearAllRefs()
			cursor.SetBlock(value, true)
			if err := tx.SetCursorAtKey(ctx, key, cursor, false); err != nil {
				t.Fatal(err)
			}
		}
	})

	// Build the replacement root from the streamed blocks.
	got := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		if err := tx.ReplaceAllBlocks(ctx, values); err != nil {
			t.Fatal(err)
		}
	})

	// Require block replacement to match the incremental root.
	if !want.EqualsRef(got) {
		t.Fatal("streamed blocks differ from incremental tree")
	}

	// Follow the last example value in the published replacement tree.
	read := openOkraRoot(t, ctx, store, got, false)
	defer read.Discard()
	cursor, err := read.GetCursorAtKey(ctx, []byte("key-0511"))
	if err != nil {
		t.Fatal(err)
	}

	// Decode the example block reached through its value cursor.
	value, err := block.UnmarshalBlock[*block_mock.Example](ctx, cursor, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err)
	}

	// Require the published block to retain its expected message.
	if value.GetMsg() != "key-0511" {
		t.Fatalf("value = %q", value.GetMsg())
	}
}
