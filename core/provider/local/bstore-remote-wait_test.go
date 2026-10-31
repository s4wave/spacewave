package provider_local

import (
	"context"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// openedBackend is an in-memory backend store.
type openedBackend struct {
	block.StoreOps
}

// Reclaim drops nothing.
func (openedBackend) Reclaim(context.Context, func(context.Context) error, func(context.Context, []*block.BlockRef) ([]bool, error)) (time.Time, error) {
	return time.Time{}, nil
}

// Close does nothing.
func (openedBackend) Close() {}

// TestRemoteReadsWaitForOpen checks that a read of a placed store waits for
// its backend to open instead of failing over to peers.
func TestRemoteReadsWaitForOpen(t *testing.T) {
	// Place a store on a backend that has not opened.
	ctx := t.Context()
	tkr := &bstoreTracker{}
	p := &placementState{}
	if tkr.getRemoteReads(p) != nil {
		t.Fatal("unplaced store reads a backend")
	}
	p.backend.Store(&account_settings.StorageBackend{})
	reads := tkr.getRemoteReads(p)
	if reads == nil {
		t.Fatal("placed store reads no backend")
	}

	// Open the backend holding a block after the read starts.
	store := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	ref, _, err := store.PutBlock(ctx, []byte("evicted"), nil)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(100*time.Millisecond, func() {
		tkr.remote.Store(&openBackend{store: openedBackend{StoreOps: store}})
	})

	// The read returns the block from the opened backend.
	data, found, err := reads.GetBlock(ctx, ref)
	if err != nil || !found || string(data) != "evicted" {
		t.Fatalf("read = %q/%v/%v", data, found, err)
	}
}
