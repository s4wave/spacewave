package store_kvtx

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hash "github.com/s4wave/spacewave/net/hash"
)

type kvtxBlockTestStore struct {
	block.StoreOps

	putCalls         int
	rmCalls          int
	batchCalls       int
	existsBatchCalls int
	beginCalls       int
	endCalls         int
}

func newKVTxBlockTestStore() *kvtxBlockTestStore {
	return &kvtxBlockTestStore{
		StoreOps: block_store_inmem.NewInmemBlock(
			store_kvkey.NewDefaultKVKey(),
			store_kvtx_inmem.NewStore(),
			hash.HashType_HashType_BLAKE3,
			false,
		),
	}
}

func (s *kvtxBlockTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putCalls++
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *kvtxBlockTestStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	s.rmCalls++
	return s.StoreOps.RmBlock(ctx, ref)
}

func (s *kvtxBlockTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.batchCalls++
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *kvtxBlockTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

func (s *kvtxBlockTestStore) BeginDeferFlush() {
	s.beginCalls++
	block.BeginDeferFlush(s.StoreOps)
}

func (s *kvtxBlockTestStore) EndDeferFlush(ctx context.Context) error {
	s.endCalls++
	return block.EndDeferFlush(ctx, s.StoreOps)
}

func TestKVTxForwardsBlockStoreExtensions(t *testing.T) {
	// Prepare a KVTx wrapper and a block reference for forwarding checks.
	ctx := context.Background()
	inner := newKVTxBlockTestStore()
	k := &KVTx{blk: inner}
	ref, err := block.BuildBlockRef([]byte("hello"), &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write a block batch through the KVTx wrapper.
	if err := k.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: []byte("hello")}}); err != nil {
		t.Fatal(err.Error())
	}

	// Require the batch write to reach the inner store without per-entry puts.
	if inner.batchCalls != 1 || inner.putCalls != 0 {
		t.Fatalf("expected one batch call and no per-entry fallback, got batch=%d put=%d", inner.batchCalls, inner.putCalls)
	}

	// Write one block through the KVTx wrapper.
	if _, _, err := k.PutBlock(ctx, []byte("hello"), &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}

	// Require the single-block write to reach the inner store.
	if inner.putCalls != 1 {
		t.Fatalf("expected one put call, got %d", inner.putCalls)
	}

	// Query block existence as a batch through the KVTx wrapper.
	if _, err := k.GetBlockExistsBatch(ctx, []*block.BlockRef{ref}); err != nil {
		t.Fatal(err.Error())
	}

	// Require the existence batch to reach the inner store.
	if inner.existsBatchCalls != 1 {
		t.Fatalf("expected batch exists forwarding, got %d calls", inner.existsBatchCalls)
	}

	// Begin and end a deferred flush through the KVTx wrapper.
	k.BeginDeferFlush()
	if err := k.EndDeferFlush(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Require both deferred-flush operations to reach the inner store.
	if inner.beginCalls != 1 || inner.endCalls != 1 {
		t.Fatalf("expected defer-flush forwarding, got begin=%d end=%d", inner.beginCalls, inner.endCalls)
	}
}

func TestConfigResolveHashType(t *testing.T) {
	// Require an absent configuration to select the default block hash.
	var nilConfig *Config
	if got := nilConfig.ResolveHashType(); got != block.DefaultHashType {
		t.Fatalf("expected nil config to resolve SHA256, got %s", got)
	}

	// Require a zero configuration to select the default block hash.
	if got := (&Config{}).ResolveHashType(); got != block.DefaultHashType {
		t.Fatalf("expected zero config to resolve SHA256, got %s", got)
	}

	// Require an explicit configuration to retain its chosen block hash.
	if got := (&Config{HashType: hash.HashType_HashType_SHA256}).ResolveHashType(); got != hash.HashType_HashType_SHA256 {
		t.Fatalf("expected explicit SHA256 config to win, got %s", got)
	}
}
