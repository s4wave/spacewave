package status

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
	"github.com/sirupsen/logrus"
)

// TestControllersResponseOpensHeldStore checks that a block store controller
// holding an overlay reports the overlay's lower and upper stores.
func TestControllersResponseOpensHeldStore(t *testing.T) {
	// Start a bus.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(cdc.NewController(ctx, le))

	// Run a block store controller over an overlay of two stores.
	overlay := block.NewOverlay(
		ctx,
		le,
		block_mock.NewMockStore(0),
		block_mock.NewMockStore(0),
		block.OverlayMode_UPPER_WRITE_CACHE,
		0,
		nil,
	)
	ctrl := block_store_controller.NewController(
		le,
		controller.NewInfo("test/bstore", controller.MustParseVersion("0.0.1"), "test block store"),
		block_store_controller.NewBlockStoreBuilder(block_store.NewStore("test/store", overlay)),
		[]string{"test/store"},
		true,
		nil,
		false,
		false,
	)
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Wait for the controller to build its store, then read the response.
	if _, err := ctrl.WaitHeldStore(ctx, nil); err != nil {
		t.Fatal(err)
	}
	var store *s4wave_status.StoreInfo
	for _, info := range buildControllersResponse(b.GetControllers()).GetControllers() {
		if info.GetId() == "test/bstore" {
			store = info.GetStore()
		}
	}
	if store == nil {
		t.Fatal("held store not reported")
	}

	// The store opens into the overlay, and the overlay into its two stores.
	if store.GetId() != "test/store" || len(store.GetParts()) != 1 {
		t.Fatalf("top store = %v", store)
	}
	inner := store.GetParts()[0]
	if inner.GetLabel() != "inner" || inner.GetType() != "*block.StoreOverlay" {
		t.Fatalf("inner store = %v", inner)
	}
	parts := inner.GetParts()
	if len(parts) != 2 || parts[0].GetLabel() != "lower" || parts[1].GetLabel() != "upper" {
		t.Fatalf("overlay parts = %v", parts)
	}
}
