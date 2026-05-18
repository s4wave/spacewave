package provider_local

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

type batchForwardTestStore struct {
	block_store.Store
	putBlockBatchHits int
	backgroundHits    int
	existsBatchHits   int
}

func newBatchForwardTestStore() *batchForwardTestStore {
	ops := block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		0,
		false,
	)
	return &batchForwardTestStore{
		Store: block_store.NewStore("test", ops),
	}
}

func (s *batchForwardTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.putBlockBatchHits++
	return s.Store.PutBlockBatch(ctx, entries)
}

func (s *batchForwardTestStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundHits++
	return s.Store.PutBlockBackground(ctx, data, opts)
}

func (s *batchForwardTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchHits++
	return s.Store.GetBlockExistsBatch(ctx, refs)
}

var (
	_ block_store.Store = ((*batchForwardTestStore)(nil))
	_ block.StoreOps    = ((*batchForwardTestStore)(nil))
)

func TestBlockStoreForwardsBatchAndBackground(t *testing.T) {
	ctx := context.Background()
	inner := newBatchForwardTestStore()
	store := &BlockStore{store: inner}
	batchData := []byte("batch")
	batchRef, err := block.BuildBlockRef(batchData, &block.PutOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: batchRef, Data: batchData}}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}
	if inner.putBlockBatchHits != 1 {
		t.Fatalf("expected 1 PutBlockBatch call, got %d", inner.putBlockBatchHits)
	}

	if _, _, err := store.PutBlockBackground(ctx, []byte("hello"), nil); err != nil {
		t.Fatalf("PutBlockBackground failed: %v", err)
	}
	if inner.backgroundHits != 1 {
		t.Fatalf("expected 1 PutBlockBackground call, got %d", inner.backgroundHits)
	}

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{batchRef}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchHits != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}
}

func TestBlockStoreReadOperationSharesDecodedBlockCache(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	store := &BlockStore{
		store:         newBatchForwardTestStore(),
		decodedBlocks: decodedBlocks,
	}
	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()

	scopedStore, ok := scoped.(*BlockStore)
	if !ok {
		t.Fatalf("scoped store type = %T, want *BlockStore", scoped)
	}
	if scopedStore.GetDecodedBlockCache() != decodedBlocks {
		t.Fatal("scoped read operation did not borrow block-store decoded cache")
	}
}
