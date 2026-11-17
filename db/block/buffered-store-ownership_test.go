package block

import (
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

func TestBufferedStoreRetainsNewDependenciesForExistingBytes(t *testing.T) {
	// Create a buffer and queue its first referenced child.
	ctx := t.Context()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)
	first, _, err := store.PutBlock(ctx, []byte("first"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Queue a second child and a parent referencing the first.
	second, _, err := store.PutBlock(ctx, []byte("second"), nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, _, err := store.PutBlock(ctx, []byte("parent"), &PutOpts{Refs: []*BlockRef{first}})
	if err != nil {
		t.Fatal(err)
	}

	// Merge the second dependency into the parent.
	if _, _, err := store.PutBlock(ctx, []byte("parent"), &PutOpts{Refs: []*BlockRef{second}}); err != nil {
		t.Fatal(err)
	}

	// Merge several dependencies, ignoring duplicate and absent requests.
	third, _, err := store.PutBlock(ctx, []byte("third"), nil)
	if err != nil {
		t.Fatal(err)
	}
	fourth, _, err := store.PutBlock(ctx, []byte("fourth"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, refs := range [][]*BlockRef{
		{first, second, third, nil, third},
		{third, first},
		{third, fourth},
	} {
		if _, _, err := store.PutBlock(ctx, []byte("parent"), &PutOpts{Refs: refs}); err != nil {
			t.Fatal(err)
		}
	}

	// Flush only the parent and the dependencies its merged references retain.
	if _, err := store.SyncReachable(ctx, parent); err != nil {
		t.Fatal(err)
	}

	// Verify every dependency survives, including later multi-reference puts.
	key, err := marshalRefKey(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := inner.recordTargets[key]; got != 4 {
		t.Fatalf("later dependency was dropped: recorded=%d want=4", got)
	}
	for _, ref := range []*BlockRef{first, second, third, fourth} {
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
