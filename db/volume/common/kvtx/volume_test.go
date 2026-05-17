package kvtx

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

type countingBatchStore struct {
	block.StoreOps

	putCalls         int
	putBatchCalls    int
	existsBatchCalls int
}

func newCountingBatchStore() *countingBatchStore {
	return &countingBatchStore{
		StoreOps: block_store_inmem.NewInmemBlock(
			store_kvkey.NewDefaultKVKey(),
			store_kvtx_inmem.NewStore(),
			0,
			false,
		),
	}
}

func (s *countingBatchStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putCalls++
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *countingBatchStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.putBatchCalls++
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *countingBatchStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

var _ block.StoreOps = ((*countingBatchStore)(nil))

func TestVolumeForwardsBatchPut(t *testing.T) {
	ctx := context.Background()
	kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatalf("NewKVKey failed: %v", err)
	}

	inner := newCountingBatchStore()
	vol, err := NewVolumeWithBlockStore(
		ctx,
		"hydra/test-volume",
		kvKey,
		store_kvtx_inmem.NewStore(),
		inner,
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolumeWithBlockStore failed: %v", err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	ref1, err := block.BuildBlockRef([]byte("hello"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}
	ref2, err := block.BuildBlockRef([]byte("world"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}

	if err := vol.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: ref1, Data: []byte("hello")},
		{Ref: ref2, Data: []byte("world")},
	}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}

	if inner.putBatchCalls != 1 {
		t.Fatalf("expected 1 PutBlockBatch call, got %d", inner.putBatchCalls)
	}
	if inner.putCalls != 0 {
		t.Fatalf("expected 0 fallback PutBlock calls, got %d", inner.putCalls)
	}

	if _, err := vol.GetBlockExistsBatch(ctx, []*block.BlockRef{ref1, ref2}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchCalls != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchCalls)
	}
}
