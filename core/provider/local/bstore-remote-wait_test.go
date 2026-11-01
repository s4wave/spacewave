package provider_local

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// openedBackend is an in-memory backend store.
type openedBackend struct {
	// StoreOps serves reads through the real in-memory store.
	block.StoreOps
	// close observes backend disposal when set.
	close func()
}

// Reclaim drops nothing.
func (openedBackend) Reclaim(context.Context, func(context.Context) error, func(context.Context, []*block.BlockRef) ([]bool, error)) (time.Time, error) {
	return time.Time{}, nil
}

// Close reports backend disposal when the test observes it.
func (b openedBackend) Close() {
	if b.close != nil {
		b.close()
	}
}

// TestRemoteReadsWaitForOpen checks that a read of a placed store waits for
// its backend to open instead of failing over to peers.
func TestRemoteReadsWaitForOpen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Place a mounted store on a backend that has not opened.
		mount, unmount := context.WithCancel(t.Context())
		defer unmount()
		ctx := t.Context()
		tkr := &bstoreTracker{}
		p := &placementState{done: mount.Done()}
		if tkr.getRemoteReads(p) != nil {
			t.Fatal("unplaced store reads a backend")
		}

		// Select a backend before its handle opens.
		backend := &account_settings.StorageBackend{Id: "first"}
		tkr.setPlacementBackend(p, backend)
		reads := tkr.getRemoteReads(p)
		if reads == nil {
			t.Fatal("placed store has no pending backend")
		}

		// Arm several readers, including one with an acquired read scope.
		store := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
		ref, _, err := store.PutBlock(ctx, []byte("evicted"), nil)
		if err != nil {
			t.Fatal(err)
		}
		results := make(chan error, 3)
		for i := range 3 {
			go func() {
				// Read through a scope for one reader and the direct store for others.
				reader := reads
				if i == 0 {
					scoped, release, err := reader.BeginReadOperation(ctx)
					if err != nil {
						results <- err
						return
					}
					defer release()
					reader = scoped
				}
				data, found, err := reader.GetBlock(ctx, ref)
				if err == nil && (!found || string(data) != "evicted") {
					err = errors.New("opened backend did not serve the evicted block")
				}
				results <- err
			}()
		}
		synctest.Wait()
		if len(results) != 0 {
			t.Fatal("reader passed an unopened backend")
		}

		// Publishing the handle wakes every waiter without a polling tick.
		closed := false
		tkr.swapRemote(&openBackend{backend: backend, store: openedBackend{StoreOps: store, close: func() {
			// Re-enter notification capture to establish that Close runs unlocked.
			_, _ = tkr.readBackend(p)
			closed = true
		}}})
		synctest.Wait()
		for range 3 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}

		// A changed placement waits for its own backend, not the old handle.
		next := &account_settings.StorageBackend{Id: "second"}
		tkr.setPlacementBackend(p, next)
		pending := tkr.getRemoteReads(p)
		go func() {
			_, _, err := pending.GetBlock(ctx, ref)
			results <- err
		}()
		synctest.Wait()
		if len(results) != 0 {
			t.Fatal("replacement read used the previous backend")
		}

		// Opening the replacement releases the old handle and the pending read.
		tkr.swapRemote(&openBackend{backend: next, store: openedBackend{StoreOps: store}})
		if !closed {
			t.Fatal("previous backend was not closed")
		}
		synctest.Wait()
		if err := <-results; err != nil {
			t.Fatal(err)
		}

		// A retry retains cancellation and removal wakes a pending reader.
		tkr.swapRemote(nil)
		pending = tkr.getRemoteReads(p)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, _, err := pending.GetBlock(cancelled, ref); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled read = %v", err)
		}

		// Removing the placement wakes a reader still waiting on its retry.
		go func() {
			_, _, err := pending.GetBlock(ctx, ref)
			results <- err
		}()
		synctest.Wait()
		tkr.setPlacementBackend(p, nil)
		synctest.Wait()
		if err := <-results; !errors.Is(err, block.ErrUnavailable) {
			t.Fatalf("removed placement = %v", err)
		}

		// Ending a mount wakes readers whose caller context remains alive.
		tkr.setPlacementBackend(p, next)
		pending = tkr.getRemoteReads(p)
		go func() {
			_, _, err := pending.GetBlock(ctx, ref)
			results <- err
		}()
		synctest.Wait()
		unmount()
		synctest.Wait()
		if err := <-results; !errors.Is(err, block.ErrUnavailable) {
			t.Fatalf("unmounted backend = %v", err)
		}

		// The original deadline still bounds a backend that never opens.
		p = &placementState{}
		tkr.setPlacementBackend(p, next)
		if _, _, err := tkr.getRemoteReads(p).GetBlock(ctx, ref); !errors.Is(err, block.ErrUnavailable) {
			t.Fatalf("unopened backend deadline = %v", err)
		}
	})
}
