package provider_local

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hydra_volume "github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	bifrost_hash "github.com/s4wave/spacewave/net/hash"
)

type countingBatchStore struct {
	block.NopStoreOps
	putCalls      int
	putBatchCalls int
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

func (s *countingBatchStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	s.putBatchCalls++
	return nil
}

// _ is a type assertion
var _ block.StoreOps = ((*countingBatchStore)(nil))

func TestVolumeBlockStoreOverlayUsesBatchPutBlock(t *testing.T) {
	ctx := context.Background()

	kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatalf("NewKVKey failed: %v", err)
	}

	kvStore := store_kvtx_inmem.NewStore()
	lowerBlocks := &countingBatchStore{}
	baseVol, err := common_kvtx.NewVolumeWithBlockStore(
		ctx,
		"alpha/test-volume",
		kvKey,
		kvStore,
		lowerBlocks,
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolumeWithBlockStore failed: %v", err)
	}
	t.Cleanup(func() { _ = baseVol.Close() })

	upperBlocks := &countingBatchStore{}
	overlay := block.NewOverlay(
		ctx,
		baseVol,
		upperBlocks,
		block.OverlayMode_UPPER_READ_CACHE,
		0,
		nil,
	)
	wrapped := hydra_volume.NewVolumeBlockStore(baseVol, overlay)

	ref1, err := block.BuildBlockRef([]byte("hello"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}
	ref2, err := block.BuildBlockRef([]byte("world"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}

	if err := wrapped.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: ref1, Data: []byte("hello")},
		{Ref: ref2, Data: []byte("world")},
	}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}

	if upperBlocks.putBatchCalls != 0 || upperBlocks.putCalls != 0 {
		t.Fatalf("upper store should be unused in UPPER_READ_CACHE writes: batch=%d put=%d", upperBlocks.putBatchCalls, upperBlocks.putCalls)
	}
	if lowerBlocks.putBatchCalls != 1 {
		t.Fatalf("expected lower block store batch path to be preserved, got %d batch calls", lowerBlocks.putBatchCalls)
	}
	if lowerBlocks.putCalls != 0 {
		t.Fatalf("expected no singleton PutBlock fallbacks, got %d", lowerBlocks.putCalls)
	}
}

func TestGCStoreOpsPreservesWrappedLowerBatchPath(t *testing.T) {
	ctx := context.Background()

	kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatalf("NewKVKey failed: %v", err)
	}

	kvStore := store_kvtx_inmem.NewStore()
	lowerBlocks := &countingBatchStore{}
	baseVol, err := common_kvtx.NewVolumeWithBlockStore(
		ctx,
		"alpha/test-volume",
		kvKey,
		kvStore,
		lowerBlocks,
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolumeWithBlockStore failed: %v", err)
	}
	t.Cleanup(func() { _ = baseVol.Close() })

	upperBlocks := &countingBatchStore{}
	overlay := block.NewOverlay(
		ctx,
		baseVol,
		upperBlocks,
		block.OverlayMode_UPPER_READ_CACHE,
		0,
		nil,
	)
	wrapped := hydra_volume.NewVolumeBlockStore(baseVol, overlay)
	gcOps := block_gc.NewGCStoreOpsWithParentAndTraceTask(
		wrapped,
		nil,
		"bucket/test",
		block_gc.BucketFlushTask(),
	)

	ref1, err := block.BuildBlockRef([]byte("hello"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}
	ref2, err := block.BuildBlockRef([]byte("world"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}

	if err := gcOps.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: ref1, Data: []byte("hello")},
		{Ref: ref2, Data: []byte("world")},
	}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}

	if upperBlocks.putBatchCalls != 0 || upperBlocks.putCalls != 0 {
		t.Fatalf("upper store should be unused in UPPER_READ_CACHE writes: batch=%d put=%d", upperBlocks.putBatchCalls, upperBlocks.putCalls)
	}
	if lowerBlocks.putBatchCalls != 1 {
		t.Fatalf("expected lower block store batch path to be preserved through GCStoreOps, got %d batch calls", lowerBlocks.putBatchCalls)
	}
	if lowerBlocks.putCalls != 0 {
		t.Fatalf("expected no singleton PutBlock fallbacks through GCStoreOps, got %d", lowerBlocks.putCalls)
	}
}
