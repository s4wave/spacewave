package block_store_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hash "github.com/s4wave/spacewave/net/hash"
)

type readScopeTestStore struct {
	block.StoreOps
	beginCalls  *atomic.Int32
	scopedCalls *atomic.Int32
	scoped      bool
}

func (s *readScopeTestStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	s.beginCalls.Add(1)
	return &readScopeTestStore{
		StoreOps:    s.StoreOps,
		beginCalls:  s.beginCalls,
		scopedCalls: s.scopedCalls,
		scoped:      true,
	}, func() {}, nil
}

func (s *readScopeTestStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if s.scoped {
		s.scopedCalls.Add(1)
	}
	return s.StoreOps.GetBlock(ctx, ref)
}

func TestStoreReadThroughScopesLowerWhenPrimaryUnavailable(t *testing.T) {
	ctx := context.Background()
	inner := block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hash.HashType_HashType_BLAKE3,
		false,
	)
	beginCalls := &atomic.Int32{}
	scopedCalls := &atomic.Int32{}
	lower := &readScopeTestStore{
		StoreOps:    inner,
		beginCalls:  beginCalls,
		scopedCalls: scopedCalls,
	}
	data := []byte("scoped lower")
	ref, _, err := lower.PutBlock(ctx, data, &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3})
	if err != nil {
		t.Fatal(err)
	}

	store := block_store.NewStoreReadThrough(
		func() block.StoreOps { return nil },
		func() block.StoreOps { return lower },
		false,
	)
	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if beginCalls.Load() != 1 {
		t.Fatalf("lower BeginReadOperation calls = %d, want 1", beginCalls.Load())
	}
	got, found, err := scoped.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("scoped lower read = %q/%v/%v", got, found, err)
	}
	if scopedCalls.Load() != 1 {
		t.Fatalf("scoped lower GetBlock calls = %d, want 1", scopedCalls.Load())
	}
}

type wrapperBatchTestStore struct {
	block.StoreOps

	putCalls         int
	rmCalls          int
	batchCalls       int
	existsBatchCalls int
	freshenCalls     int
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

func (s *wrapperBatchTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

func (s *wrapperBatchTestStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

func (s *wrapperBatchTestStore) EnsureDecodedBlockCacheFresh(ctx context.Context) error {
	s.freshenCalls++
	return nil
}

func TestStoreForwardsNativeOperations(t *testing.T) {
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

	if _, _, err := store.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.putCalls != 1 {
		t.Fatalf("expected one put call, got %d", inner.putCalls)
	}

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{ref}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.existsBatchCalls != 1 {
		t.Fatalf("expected one batch exists call, got %d", inner.existsBatchCalls)
	}
}

func TestStoreForwardsDecodedBlockCacheFreshness(t *testing.T) {
	ctx := context.Background()
	inner := newWrapperBatchTestStore()
	store := block_store.NewStore("test", inner)
	freshener, ok := store.(block.DecodedBlockCacheFreshener)
	if !ok {
		t.Fatal("wrapped store does not expose decoded cache freshness")
	}

	if err := freshener.EnsureDecodedBlockCacheFresh(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if inner.freshenCalls != 1 {
		t.Fatalf("expected one freshness call, got %d", inner.freshenCalls)
	}

	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()
	scopedFreshener, ok := scoped.(block.DecodedBlockCacheFreshener)
	if !ok {
		t.Fatal("scoped wrapped store does not expose decoded cache freshness")
	}

	if err := scopedFreshener.EnsureDecodedBlockCacheFresh(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if inner.freshenCalls != 2 {
		t.Fatalf("expected scoped freshness call, got %d", inner.freshenCalls)
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
