package cdn_bstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestRootBlockStoresPublishAgedPointerRefresh verifies that a block read past
// the pointer TTL publishes the re-fetched pointer to WaitPointer on both the
// owning and the supplied store, so a World head over either store follows a
// root that replaced its packs.
func TestRootBlockStoresPublishAgedPointerRefresh(t *testing.T) {
	// Serve an initial root pointer for the Space.
	ctx := t.Context()
	oldPtr := &cdn.CdnRootPointer{SpaceId: testSpaceID}
	srv := newTestCdnServer(t, testSpaceID, encodePointer(t, oldPtr), nil)
	hs := httptest.NewServer(http.HandlerFunc(srv.handle))
	t.Cleanup(hs.Close)

	// Open the owning store and a supplied store reading through it.
	owner, err := NewCdnBlockStore(Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
		PointerTTL: time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)
	supplied, err := NewSuppliedBlockStore(SuppliedOptions{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
		PointerTTL: time.Nanosecond,
		Store:      block_store.NewStore(testSpaceID, owner),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fetch the initial pointer into both stores.
	for _, store := range []RootBlockStore{owner, supplied} {
		if _, err := store.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Publish a root that replaced the Space's packs.
	newPtr := &cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Packs:   []*packfile.PackfileEntry{{Id: "01kcdnpack0000000000000009"}},
	}
	srv.pointer = encodePointer(t, newPtr)
	time.Sleep(time.Millisecond)

	// A read through each store wakes its pointer watcher with the new root.
	ref, err := block.BuildBlockRef([]byte("absent"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []RootBlockStore{owner, supplied} {
		if _, err := store.GetBlockExists(ctx, ref); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		got, err := store.WaitPointer(waitCtx, oldPtr)
		cancel()
		if err != nil {
			t.Fatalf("%T WaitPointer: %v", store, err)
		}
		if !got.EqualVT(newPtr) {
			t.Fatalf("%T pointer = %v, want %v", store, got, newPtr)
		}
	}
}
