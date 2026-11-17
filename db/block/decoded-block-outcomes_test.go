package block_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestDecodedBlockCacheOutcomes checks the per-outcome lookup counters and
// the per-reason not-stored counters: a first read is absent, a second hits,
// a read after a durable removal is stale_removed and counted as a miss, not
// a hit, and reads a durable store did not mark, or of staged content, count
// as not_durable and staged.
func TestDecodedBlockCacheOutcomes(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer decodedBlocks.Close()
	read := func(ops block.StoreOps, ref *block.BlockRef) error {
		tx, cursor := block.NewTransaction(ops, nil, ref, nil)
		tx.SetDecodedBlockCache(decodedBlocks)
		_, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock)
		return err
	}
	lookups := func() [block.DecodedBlockLookupOutcomeCount]uint64 {
		return decodedBlocks.Snapshot().Lookups
	}

	// A first read is absent and stores; a second hits.
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "counted"})
	if err != nil {
		t.Fatal(err)
	}
	if err := read(store, ref); err != nil {
		t.Fatal(err)
	}
	if got := lookups()[block.DecodedBlockLookupAbsent]; got != 1 {
		t.Fatalf("absent lookups %d, want 1", got)
	}
	if err := read(store, ref); err != nil {
		t.Fatal(err)
	}
	snap := decodedBlocks.Snapshot()
	if snap.Lookups[block.DecodedBlockLookupHit] != 1 || snap.Hits != 1 {
		t.Fatalf("hit lookups %d hits %d, want 1 and 1", snap.Lookups[block.DecodedBlockLookupHit], snap.Hits)
	}

	// A read after the removal finds the stale entry, a miss and not a hit.
	if err := store.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := read(store, ref); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("read after removal: %v", err)
	}
	snap = decodedBlocks.Snapshot()
	if got := snap.Lookups[block.DecodedBlockLookupStaleRemoved]; got != 1 {
		t.Fatalf("stale_removed lookups %d, want 1", got)
	}
	if snap.Hits != 1 || snap.Misses != 2 {
		t.Fatalf("hits %d misses %d after the stale lookup, want 1 and 2", snap.Hits, snap.Misses)
	}

	// A read of a block no durable store marked is not_durable.
	overlay := &pendingOverlay{StoreOps: store, pending: make(map[string][]byte)}
	pending, _, err := block.PutBlock(ctx, overlay, &block_mock.Example{Msg: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if err := read(overlay, pending); err != nil {
		t.Fatal(err)
	}
	if got := decodedBlocks.Snapshot().NotStored[block.DecodedBlockNotStoredNotDurable]; got != 1 {
		t.Fatalf("not_durable %d, want 1", got)
	}

	// A read of staged content is staged.
	staging, cursor := block.NewTransaction(store, nil, nil, nil)
	staging.SetDecodedBlockCache(decodedBlocks)
	staged, _, err := block.PutBlock(ctx, staging.StageWrites(ctx, store), &block_mock.Example{Msg: "staged"})
	if err != nil {
		t.Fatal(err)
	}
	cursor.SetRefAtCursor(staged, true)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err)
	}
	if got := decodedBlocks.Snapshot().NotStored[block.DecodedBlockNotStoredStaged]; got != 1 {
		t.Fatalf("staged %d, want 1", got)
	}

	// The labels are the exported metric names.
	if got := block.DecodedBlockLookupStaleRemoved.String(); got != "stale_removed" {
		t.Fatalf("label %q", got)
	}
	if got := block.DecodedBlockNotStoredCostUnknown.String(); got != "cost_unknown" {
		t.Fatalf("label %q", got)
	}
}
