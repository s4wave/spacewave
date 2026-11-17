package block_store_writeback

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// failAfterBatch is a local store whose batches apply and then fail, as a
// batch that commits its tombstones before a later part fails does.
type failAfterBatch struct {
	block.StoreOps
}

// PutBlockBatch applies the batch, then fails.
func (f *failAfterBatch) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	if _, err := f.StoreOps.PutBlockBatch(ctx, entries); err != nil {
		return nil, err
	}
	return nil, errors.New("a later part of the batch failed")
}

// TestPackedStoreLogNotAdmittedAfterLocalRemoval reads a block the packahead
// Log holds durably through a PackedStore with a shared decoded-block cache,
// after the local store removed the block without the PackedStore forgetting
// the Log's copy: once in a tombstone batch that applied and then failed, and
// once by a removal that bypasses the PackedStore, as Volume GC does, and
// reports itself. The Log still serves its copy, and no read may admit it to
// the pool, since no later removal report could reject it.
func TestPackedStoreLogNotAdmittedAfterLocalRemoval(t *testing.T) {
	ctx := t.Context()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer decodedBlocks.Close()

	// open puts a block in the Log and in the local store, and returns a
	// PackedStore over local.
	open := func(msg string, wrap func(block.StoreOps) block.StoreOps) (*PackedStore, block.StoreOps, *block.BlockRef) {
		t.Helper()
		log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(log.Close)
		data, err := (&block_mock.Example{Msg: msg}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		local := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
		ref, _, err := local.PutBlock(ctx, data, nil)
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
		return NewPackedStore(log, wrap(local), false), local, ref
	}

	// notAdmitted reads ref twice and checks neither read stored it into the
	// pool nor hit it there.
	notAdmitted := func(s *PackedStore, ref *block.BlockRef, why string) {
		t.Helper()
		for i := range 2 {
			before := decodedBlocks.Snapshot()
			rctx, counter := block.WithReadCounter(ctx)
			tx, cursor := block.NewTransaction(s, nil, ref, nil)
			tx.SetDecodedBlockCache(decodedBlocks)
			if _, err := cursor.Unmarshal(rctx, block_mock.NewExampleBlock); err != nil {
				t.Fatal(err)
			}
			if n := counter.Snapshot().BlockReadCount; n != 1 {
				t.Fatalf("read %d after %s took %d store reads, want 1", i, why, n)
			}
			after := decodedBlocks.Snapshot()
			if after.Hits != before.Hits || after.Stores != before.Stores {
				t.Fatalf("read %d after %s admitted the Log's copy: hits %d -> %d, stores %d -> %d", i, why, before.Hits, after.Hits, before.Stores, after.Stores)
			}
		}
	}

	// A tombstone batch that applies and then fails.
	s, _, ref := open("tombstoned in a failed batch", func(local block.StoreOps) block.StoreOps {
		return &failAfterBatch{StoreOps: local}
	})
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}}); err == nil {
		t.Fatal("the failing batch succeeded")
	}
	block.InvalidateRemovedBlock(ref)
	notAdmitted(s, ref, "a failed tombstone batch")

	// A removal past the PackedStore, which reports itself.
	s, local, ref := open("removed past the PackedStore", func(local block.StoreOps) block.StoreOps { return local })
	if err := local.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	block.InvalidateRemovedBlock(ref)
	notAdmitted(s, ref, "a removal past the PackedStore")
}
