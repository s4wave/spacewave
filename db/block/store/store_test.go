package block_store_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hash "github.com/s4wave/spacewave/net/hash"
)

type wrapperBatchTestStore struct {
	block.StoreOps

	putCalls         int
	rmCalls          int
	batchCalls       int
	backgroundCalls  int
	existsBatchCalls int
}

func newWrapperBatchTestStore() *wrapperBatchTestStore {
	return &wrapperBatchTestStore{
		StoreOps: block_store_inmem.NewInmemBlock(
			store_kvkey.NewDefaultKVKey(),
			store_kvtx_inmem.NewStore(),
			hash.HashType_HashType_BLAKE3,
			false,
		),
	}
}

func (s *wrapperBatchTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putCalls++
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *wrapperBatchTestStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	s.rmCalls++
	return s.StoreOps.RmBlock(ctx, ref)
}

func (s *wrapperBatchTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.batchCalls++
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *wrapperBatchTestStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundCalls++
	return s.StoreOps.PutBlockBackground(ctx, data, opts)
}

func (s *wrapperBatchTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

func TestStoreForwardsBatchAndBackground(t *testing.T) {
	ctx := context.Background()
	inner := newWrapperBatchTestStore()
	store := block_store.NewStore("test", inner)
	data := []byte("hello")
	ref := mustBuildBlockRef(t, data)

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data}}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.batchCalls != 1 || inner.putCalls != 0 {
		t.Fatalf("expected one batch call and no per-entry fallback, got batch=%d put=%d", inner.batchCalls, inner.putCalls)
	}

	if _, _, err := store.PutBlockBackground(ctx, data, &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.backgroundCalls != 1 || inner.putCalls != 0 {
		t.Fatalf("expected one background call and no foreground fallback, got background=%d put=%d", inner.backgroundCalls, inner.putCalls)
	}

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{ref}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.existsBatchCalls != 1 {
		t.Fatalf("expected one batch exists call, got %d", inner.existsBatchCalls)
	}
}

func mustBuildBlockRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3})
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}
