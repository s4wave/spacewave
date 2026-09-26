package kvtx_block_okra

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

func TestReplaceAllMatchesIncrementalTreeAndPreservesReaders(t *testing.T) {
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
	want := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for key, value := range values {
			if err := tx.Set(ctx, key, value); err != nil {
				t.Fatal(err)
			}
		}
	})
	got := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		if err := tx.Set(ctx, []byte("old"), []byte("retained")); err != nil {
			t.Fatal(err)
		}
		reader := tx.Iterate(ctx, nil, true, false)
		defer reader.Close()
		if !reader.Next() {
			t.Fatal("missing old key", reader.Err())
		}
		if err := tx.ReplaceAll(ctx, values); err != nil {
			t.Fatal(err)
		}
		value, err := reader.Value()
		if err != nil || string(value) != "retained" {
			t.Fatalf("old snapshot: %q, %v", value, err)
		}
	})
	if !want.EqualsRef(got) {
		t.Fatal("bulk replacement differs from incremental tree")
	}
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
	got := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		if err := tx.ReplaceAllBlocks(ctx, values); err != nil {
			t.Fatal(err)
		}
	})
	if !want.EqualsRef(got) {
		t.Fatal("streamed blocks differ from incremental tree")
	}

	read := openOkraRoot(t, ctx, store, got, false)
	defer read.Discard()
	cursor, err := read.GetCursorAtKey(ctx, []byte("key-0511"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := block.UnmarshalBlock[*block_mock.Example](ctx, cursor, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err)
	}
	if value.GetMsg() != "key-0511" {
		t.Fatalf("value = %q", value.GetMsg())
	}
}
