package block

import (
	"context"
	"testing"

	hash "github.com/aperturerobotics/bifrost/hash"
)

type overlayBatchTestStore struct {
	putCalls        int
	rmCalls         int
	batchCalls      int
	backgroundCalls int
}

func (s *overlayBatchTestStore) GetHashType() hash.HashType {
	return hash.HashType_HashType_BLAKE3
}

func (s *overlayBatchTestStore) PutBlock(_ context.Context, _ []byte, opts *PutOpts) (*BlockRef, bool, error) {
	s.putCalls++
	return opts.GetForceBlockRef(), false, nil
}

func (s *overlayBatchTestStore) GetBlock(context.Context, *BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *overlayBatchTestStore) GetBlockExists(context.Context, *BlockRef) (bool, error) {
	return false, nil
}

func (s *overlayBatchTestStore) StatBlock(context.Context, *BlockRef) (*BlockStat, error) {
	return nil, nil
}

func (s *overlayBatchTestStore) RmBlock(context.Context, *BlockRef) error {
	s.rmCalls++
	return nil
}

func (s *overlayBatchTestStore) PutBlockBatch(_ context.Context, _ []*PutBatchEntry) error {
	s.batchCalls++
	return nil
}

func (s *overlayBatchTestStore) PutBlockBackground(_ context.Context, _ []byte, opts *PutOpts) (*BlockRef, bool, error) {
	s.backgroundCalls++
	return opts.GetForceBlockRef(), false, nil
}

func TestStoreOverlayPutBlockBatchForwards(t *testing.T) {
	ctx := context.Background()
	lower := &overlayBatchTestStore{}
	upper := &overlayBatchTestStore{}
	overlay := NewOverlay(ctx, lower, upper, OverlayMode_UPPER_CACHE, 0, nil)
	ref := &BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{1})}
	entries := []*PutBatchEntry{{Ref: ref, Data: []byte("hello")}}

	if err := overlay.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err.Error())
	}

	if lower.batchCalls != 1 || upper.batchCalls != 1 {
		t.Fatalf("expected both stores to receive one batch call, got lower=%d upper=%d", lower.batchCalls, upper.batchCalls)
	}
	if lower.putCalls != 0 || upper.putCalls != 0 {
		t.Fatalf("expected no per-entry PutBlock fallback, got lower=%d upper=%d", lower.putCalls, upper.putCalls)
	}
}

func TestStoreOverlayPutBlockBackgroundForwards(t *testing.T) {
	ctx := context.Background()
	lower := &overlayBatchTestStore{}
	upper := &overlayBatchTestStore{}
	overlay := NewOverlay(ctx, lower, upper, OverlayMode_UPPER_ONLY, 0, nil)
	ref := &BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{2})}

	if _, _, err := overlay.PutBlockBackground(ctx, []byte("hello"), &PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}

	if upper.backgroundCalls != 1 {
		t.Fatalf("expected upper background call, got %d", upper.backgroundCalls)
	}
	if upper.putCalls != 0 {
		t.Fatalf("expected no foreground fallback PutBlock calls, got %d", upper.putCalls)
	}
}
