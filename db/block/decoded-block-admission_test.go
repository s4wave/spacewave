package block

import (
	"context"
	"testing"
)

// TestDecodedBlockCacheRemovalGenerations pins the removal half of the
// invariant at the generation level, where a test can order a read snapshot
// and a removal freely. A read whose snapshot opened before a removal is not
// admitted after it, an admitted entry misses once its block is removed, in
// every scope, and a later read of a re-stored block is admitted again.
func TestDecodedBlockCacheRemovalGenerations(t *testing.T) {
	// Create two scopes in one pool and an entry for a block.
	ctx := context.Background()
	first := newTestDecodedBlockCache(t)
	second := newDecodedBlockCacheScope(first.pool, false)
	ref, key, blk, data := newDecodedBlockCacheTestEntry(t, "removed under a reader")
	lookup := func(c *DecodedBlockCache) bool {
		_, ok, err := c.Lookup(ctx, nil, key, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		return ok
	}
	store := func(c *DecodedBlockCache, gen uint64) {
		if err := c.Store(ctx, nil, c.storeToken(key.ref), key, blk, len(data), gen, false); err != nil {
			t.Fatal(err.Error())
		}
	}

	// A snapshot opened before the removal must not admit the block after it.
	snapshot := DurableReadGeneration()
	InvalidateRemovedBlock(ref)
	store(first, snapshot)
	if lookup(first) {
		t.Fatal("read from a snapshot older than the removal was admitted")
	}

	// Entries admitted in both scopes miss once the block is removed.
	store(first, DurableReadGeneration())
	store(second, DurableReadGeneration())
	if !lookup(first) || !lookup(second) {
		t.Fatal("durable reads were not admitted")
	}
	InvalidateRemovedBlock(ref)
	if lookup(first) || lookup(second) {
		t.Fatal("entry of a removed block was served")
	}

	// A read after the block is stored again is admitted.
	store(first, DurableReadGeneration())
	if !lookup(first) {
		t.Fatal("read after the removal was not admitted")
	}

	// Removing every block of a store makes every entry miss.
	InvalidateAllRemovedBlocks()
	if lookup(first) {
		t.Fatal("entry survived the removal of every block")
	}
}

// TestDecodedBlockReadMarkAdmission checks that only a durable mark for the
// block being read admits it, that a staged mark vetoes a durable one, and
// that the oldest durable generation wins.
func TestDecodedBlockReadMarkAdmission(t *testing.T) {
	// Build the block being read and an unrelated one.
	ref, _, _, _ := newDecodedBlockCacheTestEntry(t, "read mark")
	other, _, _, _ := newDecodedBlockCacheTestEntry(t, "another block")

	// Unmarked, or marked durable for another block: not admitted.
	ctx, mark := withBlockReadMark(context.Background(), ref)
	if _, ok := mark.admission(); ok {
		t.Fatal("unmarked read admitted")
	}
	MarkDurableRead(ctx, other, 1)
	if _, ok := mark.admission(); ok {
		t.Fatal("durable read of another block admitted this one")
	}

	// Durable reads admit at the oldest generation.
	MarkDurableRead(ctx, ref, 7)
	MarkDurableRead(ctx, ref, 5)
	MarkDurableRead(ctx, ref, 9)
	if gen, ok := mark.admission(); !ok || gen != 5 {
		t.Fatalf("durable read admission gen=%d ok=%v, want 5 true", gen, ok)
	}

	// A staged read vetoes admission.
	MarkStagedRead(ctx)
	if _, ok := mark.admission(); ok {
		t.Fatal("staged read admitted")
	}
}
