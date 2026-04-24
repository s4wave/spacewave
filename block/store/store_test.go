package block_store

import (
	"context"
	"testing"

	hash "github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
)

type wrapperBatchTestStore struct {
	block.NopStoreOps

	putCalls         int
	rmCalls          int
	batchCalls       int
	backgroundCalls  int
	existsBatchCalls int
}

func (s *wrapperBatchTestStore) GetHashType() hash.HashType {
	return hash.HashType_HashType_BLAKE3
}

func (s *wrapperBatchTestStore) PutBlock(_ context.Context, _ []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putCalls++
	return opts.GetForceBlockRef(), false, nil
}

func (s *wrapperBatchTestStore) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *wrapperBatchTestStore) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *wrapperBatchTestStore) StatBlock(context.Context, *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

func (s *wrapperBatchTestStore) RmBlock(context.Context, *block.BlockRef) error {
	s.rmCalls++
	return nil
}

func (s *wrapperBatchTestStore) PutBlockBatch(_ context.Context, _ []*block.PutBatchEntry) error {
	s.batchCalls++
	return nil
}

func (s *wrapperBatchTestStore) PutBlockBackground(_ context.Context, _ []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundCalls++
	return opts.GetForceBlockRef(), false, nil
}

func (s *wrapperBatchTestStore) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return make([]bool, len(refs)), nil
}

func TestStoreForwardsBatchAndBackground(t *testing.T) {
	ctx := context.Background()
	inner := &wrapperBatchTestStore{}
	store := NewStore("test", inner)
	ref := &block.BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{1})}

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: []byte("hello")}}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.batchCalls != 1 || inner.putCalls != 0 {
		t.Fatalf("expected one batch call and no per-entry fallback, got batch=%d put=%d", inner.batchCalls, inner.putCalls)
	}

	if _, _, err := store.PutBlockBackground(ctx, []byte("hello"), &block.PutOpts{ForceBlockRef: ref}); err != nil {
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
