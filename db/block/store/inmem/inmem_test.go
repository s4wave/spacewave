package block_store_inmem

import (
	"context"
	"testing"
	"time"

	block_store_test "github.com/s4wave/spacewave/db/block/store/test"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreInmem tests the inmem block store.
func TestBlockStoreInmem(t *testing.T) {
	// Create a logger for the in-memory block store checks.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Hold a block store reference while the controller runs.
	storeID := "test-store"
	ctrl := NewController(le, &Config{BlockStoreId: storeID})
	storeProm, storeRef := ctrl.AddBlockStoreRef()
	defer storeRef.Release()

	// Run the controller to resolve the requested in-memory block store.
	go func() {
		_ = ctrl.Execute(ctx)
	}()

	// Obtain the block store client from the controller's reference.
	client, err := storeProm.Await(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the shared block store contract against the in-memory store.
	if err := block_store_test.TestAll(ctx, client, time.Millisecond*100); err != nil {
		t.Fatal(err.Error())
	}
}
