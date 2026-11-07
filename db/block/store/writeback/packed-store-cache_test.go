package block_store_writeback

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// TestPackedStoreReadsEnterDecodedPool reads a block the Log holds through
// two transactions on one decoded-block cache: the first read admits it to
// the shared pool, as a Volume read does, and the second hits. After RmBlock
// forgets the Log's copy, a read serves no pooled block.
func TestPackedStoreReadsEnterDecodedPool(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	packed := NewPackedStore(log, ownedStore{env.vol}, true)

	// Divert a block to the Log.
	data, err := block_mock.NewExample("packed example").MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ref, _, err := packed.PutBlock(ctx, data, leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := env.vol.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("Volume holds the diverted bytes: %v %v", found, err)
	}

	// read decodes ref in a fresh read-only transaction on cache.
	cache := block.NewDecodedBlockCache()
	t.Cleanup(cache.Close)
	read := func() (block.Block, error) {
		btx, cursor := block.NewTransaction(packed, nil, ref, nil)
		btx.SetDecodedBlockCache(cache)
		btx.SetReadOnly()
		return cursor.Unmarshal(ctx, block_mock.NewExampleBlock)
	}

	// The first read admits the block, and the second hits.
	if blk, err := read(); err != nil || blk.(*block_mock.Example).GetMsg() != "packed example" {
		t.Fatalf("first read = %v, %v", blk, err)
	}
	first := cache.Snapshot()
	if first.Stores == 0 {
		t.Fatalf("a Log read was not admitted to the pool: %+v", first)
	}
	if _, err := read(); err != nil {
		t.Fatal(err)
	}
	if second := cache.Snapshot(); second.Hits <= first.Hits {
		t.Fatalf("second read missed the pool: %+v", second)
	}

	// Once removed, the pooled entry is stale and the read finds nothing.
	if err := packed.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if blk, err := read(); err == nil && blk != nil && blk.(*block_mock.Example).GetMsg() != "" {
		t.Fatalf("read after RmBlock returned %v", blk)
	}
	if entries := cache.Snapshot().Entries; entries != 0 {
		t.Fatalf("the pool keeps %d entries after RmBlock", entries)
	}
}

// TestPackedStoreTombstoneForgetsLogCopy tombstones a packed block in a batch
// with the switch off: the Log forgets its copy, as RmBlock does, so the block
// no longer reads.
func TestPackedStoreTombstoneForgetsLogCopy(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)

	// Divert a block to the Log.
	ref, _, err := NewPackedStore(log, ownedStore{env.vol}, true).PutBlock(ctx, []byte("packed, then tombstoned"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}

	// Tombstone it through a store with the switch off.
	off := NewPackedStore(log, ownedStore{env.vol}, false)
	if _, err := off.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}}); err != nil {
		t.Fatal(err)
	}
	if log.Has(packfile.BlockKey(ref.GetHash())) {
		t.Fatal("the Log keeps a tombstoned block")
	}
	if _, found, err := off.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("tombstoned block reads: %v, %v", found, err)
	}
}
