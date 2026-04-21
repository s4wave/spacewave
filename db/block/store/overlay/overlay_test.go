package block_store_overlay

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_test "github.com/s4wave/spacewave/db/block/store/test"
	core_test "github.com/s4wave/spacewave/db/core/test"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreOverlay tests the overlay block store.
func TestBlockStoreOverlay(t *testing.T) {
	// Available overlay modes:
	ctx := context.Background()
	overlayModes := []block.OverlayMode{
		block.OverlayMode_UPPER_ONLY,
		block.OverlayMode_LOWER_ONLY,
		block.OverlayMode_UPPER_CACHE,
		block.OverlayMode_LOWER_CACHE,
		block.OverlayMode_UPPER_READ_CACHE,
		block.OverlayMode_LOWER_READ_CACHE,
		block.OverlayMode_UPPER_WRITE_CACHE,
		block.OverlayMode_LOWER_WRITE_CACHE,
	}

	// For each test:
	for _, overlayMode := range overlayModes {
		t.Run(overlayMode.String(), func(t *testing.T) {
			ctx, ctxCancel := context.WithCancel(ctx)
			defer ctxCancel()

			log := logrus.New()
			log.SetLevel(logrus.DebugLevel)
			le := logrus.NewEntry(log)

			// Construct bus
			b, sr, err := core_test.NewTestingBus(ctx, le)
			if err != nil {
				t.Fatal(err.Error())
			}
			sr.AddFactory(block_store_inmem.NewFactory(b))
			sr.AddFactory(NewFactory(b))

			// Construct a lower kvtx inmem store.
			lowerBlockStoreID := "store/lower"
			lowerStoreConf := block_store_inmem.NewConfig(lowerBlockStoreID, nil)
			_, _, lowerStoreRef, err := loader.WaitExecControllerRunning(
				ctx,
				b,
				resolver.NewLoadControllerWithConfig(lowerStoreConf),
				nil,
			)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer lowerStoreRef.Release()

			// Construct a upper kvtx inmem store.
			upperBlockStoreID := "store/upper"
			upperStoreConf := block_store_inmem.NewConfig(upperBlockStoreID, nil)
			_, _, upperStoreRef, err := loader.WaitExecControllerRunning(
				ctx,
				b,
				resolver.NewLoadControllerWithConfig(upperStoreConf),
				nil,
			)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer upperStoreRef.Release()

			// Construct the overlay store
			overlayBlockStoreID := "store/overlay"
			overlayStoreConf := NewConfig(overlayBlockStoreID, lowerBlockStoreID, upperBlockStoreID, overlayMode, nil)
			_, _, overlayStoreRef, err := loader.WaitExecControllerRunning(
				ctx,
				b,
				resolver.NewLoadControllerWithConfig(overlayStoreConf),
				nil,
			)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer overlayStoreRef.Release()

			// Lookup the overlay store
			overlayStore, _, overlayStoreRef, err := block_store.ExLookupFirstBlockStore(ctx, b, overlayBlockStoreID, false, nil)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer overlayStoreRef.Release()

			// Run generic tests
			if err := block_store_test.TestAll(ctx, overlayStore, time.Millisecond*100); err != nil {
				t.Fatal(err.Error())
			}
		})
	}
}
