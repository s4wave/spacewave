package kvtx

import (
	"context"
	"testing"

	bifrost_hash "github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
)

type countingBatchStore struct {
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
	_ block.StoreOps         = ((*countingBatchStore)(nil))
	_ block.BatchExistsStore = ((*countingBatchStore)(nil))
	_ block.BatchPutStore    = ((*countingBatchStore)(nil))
)

func TestVolumeForwardsBatchPutStore(t *testing.T) {
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

	batcher, ok := any(vol).(block.BatchPutStore)
	if !ok {
		t.Fatal("expected Volume to implement block.BatchPutStore")
	}

	ref1, err := block.BuildBlockRef([]byte("hello"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}
	ref2, err := block.BuildBlockRef([]byte("world"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}

	if err := batcher.PutBlockBatch(ctx, []*block.PutBatchEntry{
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

	existsBatcher, ok := any(vol).(block.BatchExistsStore)
	if !ok {
		t.Fatal("expected Volume to implement block.BatchExistsStore")
	}
	if _, err := existsBatcher.GetBlockExistsBatch(ctx, []*block.BlockRef{ref1, ref2}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchCalls != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchCalls)
	}
}
