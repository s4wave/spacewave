package kvtx_block_okra

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/tx"
)

// TestReadCursorLifetime walks eviction, old roots, private materialization,
// cache disable, cancellation and physical removal through real Okra reads.
func TestReadCursorLifetime(t *testing.T) {
	// Publish enough pages to exceed a deliberately small decoded budget.
	ctx := t.Context()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 4096)
	rootRef, _ := writeOkraFixture(t, ctx, store, fixture)
	cache, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{MaxCost: 128 << 10})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	open := func(ref *block.BlockRef, scope *block.DecodedBlockCache) *Tx {
		bt, cs := block.NewTransaction(store, nil, ref, nil)
		bt.SetReadOnly()
		bt.SetDecodedBlockCache(scope)
		tr, err := NewTx(ctx, cs, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if tr.readRoot == nil {
			t.Fatal("readonly stored root did not use immutable traversal")
		}
		return tr
	}
	old := open(rootRef, cache)
	defer old.Discard()

	// Evict and reload pages repeatedly while validating complete value bytes.
	for range 2 {
		for i, key := range fixture.keys {
			value, found, err := old.Get(ctx, key)
			if err != nil || !found || !bytes.Equal(value, fixture.values[i]) {
				t.Fatalf("read %q found=%v err=%v", key, found, err)
			}
		}
	}
	stats := cache.Snapshot()
	if stats.Evictions == 0 || stats.RetainedCost > stats.MaxCost {
		t.Fatalf("eviction budget not exercised: %+v", stats)
	}

	// A child reader owns caller refs, including edges without a memo slot.
	parent := *old.readRoot
	page, err := block.UnmarshalReadBlock[Page](ctx, parent)
	if err != nil {
		t.Fatal(err)
	}
	if page.GetLevel() == 0 {
		t.Fatal("fixture lacks a child page")
	}
	input := page.GetEntries()[0].GetChildRef().Clone()
	expected, err := block.UnmarshalReadBlock[Page](ctx, parent.FollowRef(0, input))
	if err != nil {
		t.Fatal(err)
	}
	expectedHash := append([]byte(nil), expected.GetPageHash()...)
	child := parent.FollowRef(0, input)
	input.Hash = nil
	actual, err := block.UnmarshalReadBlock[Page](ctx, child)
	if err != nil || len(expectedHash) == 0 || !bytes.Equal(actual.GetPageHash(), expectedHash) {
		t.Fatalf("caller changed cold child identity: %v", err)
	}

	// Typed child bindings retain their identity after the parent is retired.
	input = page.GetEntries()[0].GetChildRef().Clone()
	memoized := parent.FollowRef(entryChildRefID(0), input)
	input.Hash = nil
	cache.InvalidateAll(ctx)
	if _, err := block.UnmarshalReadBlock[Page](ctx, parent); err != nil {
		t.Fatal(err)
	}
	actual, err = block.UnmarshalReadBlock[Page](ctx, memoized)
	if err != nil || !bytes.Equal(actual.GetPageHash(), expectedHash) {
		t.Fatalf("child lost identity after parent retirement: %v", err)
	}

	// Change a leaf slot and its value while the original revision remains open.
	bt, cs := block.NewTransaction(store, nil, rootRef, nil)
	bt.SetDecodedBlockCache(cache)
	writer, err := NewTx(ctx, cs, bt, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()
	if err := writer.Set(ctx, fixture.keys[0], []byte("new value")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Set(ctx, []byte("key-000000-extra"), []byte("shifted slot")); err != nil {
		t.Fatal(err)
	}
	next, _, err := bt.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	fresh := open(next, cache)
	defer fresh.Discard()
	value, _, err := fresh.Get(ctx, fixture.keys[0])
	if err != nil || string(value) != "new value" {
		t.Fatalf("new revision value=%q err=%v", value, err)
	}
	value, _, err = old.Get(ctx, fixture.keys[0])
	if err != nil || !bytes.Equal(value, fixture.values[0]) {
		t.Fatalf("old revision changed: %q %v", value, err)
	}

	// Materialized cursors still read the exact stored value and reject writes.
	valueCursor, err := old.GetCursorAtKey(ctx, fixture.keys[1])
	if err != nil {
		t.Fatal(err)
	}
	data, found, err := valueCursor.Fetch(ctx)
	if err != nil || !found || !bytes.Equal(data, fixture.values[1]) {
		t.Fatalf("materialized value=%q found=%v err=%v", data, found, err)
	}
	if _, _, err := valueCursor.GetTransaction().Write(ctx, false); !errors.Is(err, tx.ErrNotWrite) {
		t.Fatalf("readonly cursor write=%v", err)
	}

	// A cancelled call cannot use an already populated immutable node.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := old.Get(cancelled, fixture.keys[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read=%v", err)
	}

	// Scope invalidation forces a verified reload without changing the old root.
	cache.InvalidateAll(ctx)
	if _, found, err := old.Get(ctx, fixture.keys[0]); err != nil || !found {
		t.Fatalf("reload after invalidation found=%v err=%v", found, err)
	}

	// Physically remove the loaded root page: retained nodes must not hide it.
	if err := store.RmBlock(ctx, old.root.GetRootPageRef()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := old.Get(ctx, fixture.keys[0]); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("removed page was served from memory: %v", err)
	}

	// Closing a cache scope makes existing readers use storage rather than slots.
	cache.Close()
	value, found, err = fresh.Get(ctx, fixture.keys[0])
	if err != nil || !found || string(value) != "new value" {
		t.Fatalf("closed-cache read=%q found=%v err=%v", value, found, err)
	}
}
