package block

import (
	"context"
	"testing"
)

type storeRWFreshener struct {
	NopStoreOps

	scoped       *storeRWFreshener
	freshenCalls int
}

func (s *storeRWFreshener) BeginReadOperation(context.Context) (StoreOps, func(), error) {
	if s.scoped != nil {
		return s.scoped, func() {}, nil
	}
	return s, func() {}, nil
}

func (s *storeRWFreshener) EnsureDecodedBlockCacheFresh(context.Context) error {
	s.freshenCalls++
	return nil
}

func TestStoreRWForwardsDecodedBlockCacheFreshness(t *testing.T) {
	ctx := context.Background()
	scopedInner := &storeRWFreshener{}
	readInner := &storeRWFreshener{scoped: scopedInner}
	store := NewStoreRW(readInner, nil)

	if err := store.(DecodedBlockCacheFreshener).EnsureDecodedBlockCacheFresh(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if readInner.freshenCalls != 1 {
		t.Fatalf("expected one read-handle freshness call, got %d", readInner.freshenCalls)
	}

	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()
	if err := scoped.(DecodedBlockCacheFreshener).EnsureDecodedBlockCacheFresh(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if scopedInner.freshenCalls != 1 {
		t.Fatalf("expected one scoped freshness call, got %d", scopedInner.freshenCalls)
	}
}
