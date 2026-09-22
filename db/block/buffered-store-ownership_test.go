package block

import (
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

func TestBufferedStoreRetainsNewDependenciesForExistingBytes(t *testing.T) {
	ctx := t.Context()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)
	first, _, err := store.PutBlock(ctx, []byte("first"), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.PutBlock(ctx, []byte("second"), nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, _, err := store.PutBlock(ctx, []byte("parent"), &PutOpts{Refs: []*BlockRef{first}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.PutBlock(ctx, []byte("parent"), &PutOpts{Refs: []*BlockRef{second}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncReachable(ctx, parent); err != nil {
		t.Fatal(err)
	}
	key, err := marshalRefKey(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := inner.recordTargets[key]; got != 2 {
		t.Fatalf("later dependency was dropped: recorded=%d want=2", got)
	}
	for _, ref := range []*BlockRef{first, second} {
		if found, err := inner.GetBlockExists(ctx, ref); err != nil || !found {
			t.Fatalf("reachable dependency discarded: %v %v", found, err)
		}
	}
	// A new destination may share physical bytes without owning them. Forward
	// its put even when the underlying existence probe finds the payload.
	before := inner.batchCalls
	if _, exists, err := store.PutBlock(ctx, []byte("parent"), &PutOpts{Refs: []*BlockRef{first}}); err != nil || !exists {
		t.Fatalf("existing put: %v %v", exists, err)
	}
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if inner.batchCalls != before+1 {
		t.Fatal("existing bytes bypassed the ownership write")
	}
}
