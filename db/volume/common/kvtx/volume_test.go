package kvtx

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	bifrost_hash "github.com/s4wave/spacewave/net/hash"
)

type countingBatchStore struct {
	block.NopStoreOps

	putCalls         int
	putBatchCalls    int
	existsBatchCalls int
}

func (s *countingBatchStore) GetHashType() bifrost_hash.HashType { return 0 }

func (s *countingBatchStore) PutBlock(_ context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putCalls++
	ref, err := block.BuildBlockRef(data, opts)
	return ref, false, err
}

func (s *countingBatchStore) GetBlock(_ context.Context, _ *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *countingBatchStore) GetBlockExists(_ context.Context, _ *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *countingBatchStore) StatBlock(_ context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return &block.BlockStat{Ref: ref}, nil
}

func (s *countingBatchStore) RmBlock(_ context.Context, _ *block.BlockRef) error {
	return nil
}

func (s *countingBatchStore) PutBlockBatch(_ context.Context, _ []*block.PutBatchEntry) error {
	s.putBatchCalls++
	return nil
}

func (s *countingBatchStore) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return make([]bool, len(refs)), nil
}

var (
	_ block.StoreOps = ((*countingBatchStore)(nil))
)

func TestVolumeForwardsBatchPut(t *testing.T) {
	ctx := context.Background()
	kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatalf("NewKVKey failed: %v", err)
	}

	inner := &countingBatchStore{}
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
