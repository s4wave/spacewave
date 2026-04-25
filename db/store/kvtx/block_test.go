package store_kvtx

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	hash "github.com/s4wave/spacewave/net/hash"
)

type kvtxBlockTestStore struct {
	block.NopStoreOps

	putCalls         int
	rmCalls          int
	batchCalls       int
	backgroundCalls  int
	existsBatchCalls int
	beginCalls       int
	endCalls         int
}

func (s *kvtxBlockTestStore) GetHashType() hash.HashType {
	return hash.HashType_HashType_BLAKE3
}

func (s *kvtxBlockTestStore) PutBlock(_ context.Context, _ []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putCalls++
	return opts.GetForceBlockRef(), false, nil
}

func (s *kvtxBlockTestStore) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *kvtxBlockTestStore) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *kvtxBlockTestStore) StatBlock(context.Context, *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

func (s *kvtxBlockTestStore) RmBlock(context.Context, *block.BlockRef) error {
	s.rmCalls++
	return nil
}

func (s *kvtxBlockTestStore) PutBlockBatch(_ context.Context, _ []*block.PutBatchEntry) error {
	s.batchCalls++
	return nil
}

func (s *kvtxBlockTestStore) PutBlockBackground(_ context.Context, _ []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundCalls++
	return opts.GetForceBlockRef(), false, nil
}

func (s *kvtxBlockTestStore) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return make([]bool, len(refs)), nil
}

func (s *kvtxBlockTestStore) BeginDeferFlush() {
	s.beginCalls++
}

func (s *kvtxBlockTestStore) EndDeferFlush(context.Context) error {
	s.endCalls++
	return nil
}

func TestKVTxForwardsBlockStoreExtensions(t *testing.T) {
	ctx := context.Background()
	inner := &kvtxBlockTestStore{}
	k := &KVTx{blk: inner}
	ref := &block.BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{1})}

	if err := k.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: []byte("hello")}}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.batchCalls != 1 || inner.putCalls != 0 {
		t.Fatalf("expected one batch call and no per-entry fallback, got batch=%d put=%d", inner.batchCalls, inner.putCalls)
	}

	if _, _, err := k.PutBlockBackground(ctx, []byte("hello"), &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.backgroundCalls != 1 || inner.putCalls != 0 {
		t.Fatalf("expected one background call and no foreground fallback, got background=%d put=%d", inner.backgroundCalls, inner.putCalls)
	}

	if _, err := k.GetBlockExistsBatch(ctx, []*block.BlockRef{ref}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.existsBatchCalls != 1 {
		t.Fatalf("expected batch exists forwarding, got %d calls", inner.existsBatchCalls)
	}

	k.BeginDeferFlush()
	if err := k.EndDeferFlush(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if inner.beginCalls != 1 || inner.endCalls != 1 {
		t.Fatalf("expected defer-flush forwarding, got begin=%d end=%d", inner.beginCalls, inner.endCalls)
	}
}
