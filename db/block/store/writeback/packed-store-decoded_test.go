package block_store_writeback

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestPackedStoreLogHitDecodedPool reads a block the packahead Log holds
// durably twice through a PackedStore with a shared decoded-block cache: the
// first read admits it to the pool and the second decodes it from the pool
// without a store read. After RmBlock the Log still serves the value until its
// pack uploads, but the next read misses the pool, reads the Log again, and
// admits nothing, since the Log no longer holds the value durably.
func TestPackedStoreLogHitDecodedPool(t *testing.T) {
	// Put a block in the Log only, and read through an empty local store.
	ctx := t.Context()
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	data, err := (&block_mock.Example{Msg: "held by the Log"}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	value, err := block.EncodeBlockObject(data, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(ctx, []packahead.Block{{Key: packfile.BlockKey(ref.GetHash()), Value: value}}); err != nil {
		t.Fatal(err)
	}
	local := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	s := NewPackedStore(log, local, true)
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer decodedBlocks.Close()

	// read reads ref in a new transaction and returns the store reads it
	// took.
	read := func() uint64 {
		t.Helper()
		rctx, counter := block.WithReadCounter(ctx)
		tx, cursor := block.NewTransaction(s, nil, ref, nil)
		tx.SetDecodedBlockCache(decodedBlocks)
		if _, err := cursor.Unmarshal(rctx, block_mock.NewExampleBlock); err != nil {
			t.Fatal(err)
		}
		return counter.Snapshot().BlockReadCount
	}

	// The first read stores the Log's block into the pool; the second hits
	// it with no store read.
	if n := read(); n != 1 {
		t.Fatalf("first read took %d store reads, want 1", n)
	}
	first := decodedBlocks.Snapshot()
	if first.Stores != 1 {
		t.Fatalf("a durable Log read stored %d blocks into the pool, want 1", first.Stores)
	}
	if n := read(); n != 0 {
		t.Fatalf("second read took %d store reads, want 0", n)
	}
	if got := decodedBlocks.Snapshot().Hits; got != first.Hits+1 {
		t.Fatalf("second read missed the pool: hits %d -> %d", first.Hits, got)
	}

	// Remove the block. Each later read misses the pool and reads the Log's
	// copy, which is readable but not durable, so it is never admitted.
	if err := s.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		before := decodedBlocks.Snapshot()
		if n := read(); n != 1 {
			t.Fatalf("read %d after RmBlock took %d store reads, want 1", i, n)
		}
		after := decodedBlocks.Snapshot()
		if after.Hits != before.Hits {
			t.Fatalf("read %d after RmBlock hit the pool: hits %d -> %d", i, before.Hits, after.Hits)
		}
		if after.Stores != before.Stores {
			t.Fatalf("read %d after RmBlock stored a removed block into the pool", i)
		}
	}
}
