//go:build !goscript

package provider_local

import (
	"context"
	"testing"
	"testing/synctest"

	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	bus_inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/sirupsen/logrus"
)

func TestLocalBlockStoreNetworkLookupUsesLocalOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Cancel the test buses when the synthetic test returns.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		le := logrus.NewEntry(logrus.New())
		mainBus := bus_inmem.NewBus(directive_controller.NewController(ctx, le))
		childBus := bus_inmem.NewBus(directive_controller.NewController(ctx, le))

		// Mount the local block store and bridge it onto the child bus.
		local := newBatchForwardTestStore()
		const storeID = "p/local/account/blk/store"
		releaseStore, err := mainBus.AddController(
			ctx,
			newLocalBlockStoreController(le, storeID, local),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseStore()
		releaseBridge, err := childBus.AddController(ctx, bus_bridge.NewBusBridge(mainBus, nil), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseBridge()

		// Build a read-through block store for a block the local owner lacks.
		ref, err := block.BuildBlockRef([]byte("missing local block"), nil)
		if err != nil {
			t.Fatal(err)
		}
		readOps := block_store.NewStoreReadThrough(
			func() block.StoreOps { return local },
			nil,
			true,
		)
		store := &BlockStore{
			store:     local,
			readStore: block_store.NewStore(storeID, readOps),
		}

		// Look up the missing block without blocking the synthetic clock.
		type lookupResult struct {
			found bool
			err   error
		}
		resultCh := make(chan lookupResult, 1)
		go func() {
			_, found, lookupErr := store.GetBlock(ctx, ref)
			resultCh <- lookupResult{found: found, err: lookupErr}
		}()
		synctest.Wait()

		// Require the lookup to miss locally instead of entering the session DEX.
		select {
		case result := <-resultCh:
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.found {
				t.Fatal("missing local block was reported as found")
			}
		default:
			cancel()
			<-resultCh
			t.Fatal("network lookup re-entered the Session DEX instead of reading the local owner")
		}
	})
}
