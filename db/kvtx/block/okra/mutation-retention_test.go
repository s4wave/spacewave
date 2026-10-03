package kvtx_block_okra

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// TestRepeatedMutationReleasesReplacedPages verifies bounded transaction memory
// while unchanged pages remain shared, then reads every value after commit.
func TestRepeatedMutationReleasesReplacedPages(t *testing.T) {
	// Open a writable tree for measuring retained transaction nodes.
	ctx := t.Context()
	store := newOkraTestStore()
	btx, cursor := block.NewTransaction(store, nil, nil, nil)
	tx, err := NewTx(ctx, cursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Populate the tree and track the expected values.
	values := make(map[string][]byte)
	for i := range 512 {
		key := "key-" + strconv.Itoa(i)
		values[key] = []byte(key)
		if err := tx.Set(ctx, []byte(key), values[key]); err != nil {
			t.Fatal(err)
		}
	}

	// Replace a small key set repeatedly after recording the initial node count.
	initial := len(btx.GetBlockGraph().Nodes())
	for i := range 2000 {
		key := "key-" + strconv.Itoa(i%7)
		values[key] = []byte(strconv.Itoa(i))
		if err := tx.Set(ctx, []byte(key), values[key]); err != nil {
			t.Fatal(err)
		}
	}

	// Require transaction nodes to remain bounded after repeated replacements.
	retained := len(btx.GetBlockGraph().Nodes())
	t.Logf("transaction nodes: %d after initial insertion, %d after replacement", initial, retained)
	if retained > 4*initial+128 {
		t.Fatalf("retained abandoned pages: %d nodes after starting with %d", retained, initial)
	}

	// Commit the tree and publish its current root.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Require every final value to read back from the published root.
	read := openOkraRoot(t, ctx, store, ref, false)
	defer read.Discard()
	for key, want := range values {
		got, found, err := read.Get(ctx, []byte(key))
		if err != nil || !found || !bytes.Equal(got, want) {
			t.Fatalf("%s = %q, want %q: %v", key, got, want, err)
		}
	}
}

// TestMutationPreservesIteratorAndValueCursor keeps readers alive while every
// page is removed, then reuses a value after closing the iterator.
func TestMutationPreservesIteratorAndValueCursor(t *testing.T) {
	// Open a writable tree for retaining readers across page removal.
	ctx := t.Context()
	store := newOkraTestStore()
	btx, root := block.NewTransaction(store, nil, nil, nil)
	tx, err := NewTx(ctx, root, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Populate the tree with values covering multiple pages.
	for i := range 512 {
		key := []byte(fmt.Sprintf("key-%03d", i))
		if err := tx.Set(ctx, key, key); err != nil {
			t.Fatal(err)
		}
	}

	// Retain a snapshot iterator and its first value cursor.
	iter := tx.BlockIterate(ctx, nil, true, false)
	defer iter.Close()
	if !iter.Next() {
		t.Fatal("missing first entry", iter.Err())
	}
	saved := iter.ValueCursor()

	// Remove every key from the current tree.
	for i := range 512 {
		if err := tx.Delete(ctx, []byte(fmt.Sprintf("key-%03d", i))); err != nil {
			t.Fatal(err)
		}
	}

	// Require the retained iterator to expose every original value.
	for i := range 512 {
		want := fmt.Sprintf("key-%03d", i)
		got, err := iter.Value()
		if !iter.Valid() || err != nil || string(got) != want {
			t.Fatalf("iterator value = %q, want %q: %v", got, want, err)
		}
		iter.Next()
	}

	// Require snapshot traversal to finish without an iterator error.
	if iter.Valid() || iter.Err() != nil {
		t.Fatalf("iterator completion: valid=%v err=%v", iter.Valid(), iter.Err())
	}

	// Require closing the iterator to release its obsolete page references.
	iter.Close()
	if nodes := len(btx.GetBlockGraph().Nodes()); nodes > 2*512+8 {
		t.Fatalf("closed iterator retained obsolete pages: %d nodes", nodes)
	}

	// Require the retained value cursor to survive iterator closure.
	if got, err := blob.FetchToBytes(ctx, saved); err != nil || string(got) != "key-000" {
		t.Fatalf("saved value = %q: %v", got, err)
	}

	// Restore the retained value cursor into the tree.
	if err := tx.SetCursorAtKey(ctx, []byte("restored"), saved, true); err != nil {
		t.Fatal(err)
	}

	// Commit and publish the restored value.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Require the restored value to survive publication.
	read := openOkraRoot(t, ctx, store, ref, false)
	defer read.Discard()
	if got, found, err := read.Get(ctx, []byte("restored")); err != nil || !found || string(got) != "key-000" {
		t.Fatalf("restored value = %q: found=%v err=%v", got, found, err)
	}
}
