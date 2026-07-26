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
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		le := logrus.NewEntry(logrus.New())
		mainBus := bus_inmem.NewBus(directive_controller.NewController(ctx, le))
		childBus := bus_inmem.NewBus(directive_controller.NewController(ctx, le))

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
