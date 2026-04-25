package provider_local

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
)

type batchForwardTestStore struct {
	block.NopStoreOps
	id                string
	putBlockBatchHits int
	backgroundHits    int
	existsBatchHits   int
}

func (s *batchForwardTestStore) GetID() string              { return s.id }
func (s *batchForwardTestStore) GetHashType() hash.HashType { return 0 }
func (s *batchForwardTestStore) PutBlock(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, nil
}

func (s *batchForwardTestStore) GetBlock(_ context.Context, _ *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *batchForwardTestStore) GetBlockExists(_ context.Context, _ *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *batchForwardTestStore) StatBlock(_ context.Context, _ *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}
func (s *batchForwardTestStore) RmBlock(_ context.Context, _ *block.BlockRef) error { return nil }
func (s *batchForwardTestStore) PutBlockBatch(_ context.Context, _ []*block.PutBatchEntry) error {
	s.putBlockBatchHits++
	return nil
}

func (s *batchForwardTestStore) PutBlockBackground(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundHits++
	return nil, false, nil
}

func (s *batchForwardTestStore) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchHits++
	return make([]bool, len(refs)), nil
}

var (
	_ block_store.Store = ((*batchForwardTestStore)(nil))
	_ block.StoreOps    = ((*batchForwardTestStore)(nil))
)

func TestBlockStoreForwardsBatchAndBackground(t *testing.T) {
	ctx := context.Background()
	inner := &batchForwardTestStore{id: "test"}
	store := &BlockStore{store: inner}

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: &block.BlockRef{}}}); err != nil {
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

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{{}}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchHits != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}
}
