package block_store_ristretto

import (
	"context"
	"testing"
	"time"

	block_store_test "github.com/s4wave/spacewave/db/block/store/test"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreRistretto tests the ristretto block store.
func TestBlockStoreRistretto(t *testing.T) {
	// Capture diagnostics for the Ristretto block store test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create a Ristretto controller and retain its block store reference.
	storeID := "test-store"
	ctrl := NewController(le, &Config{BlockStoreId: storeID})
	storeProm, storeRef := ctrl.AddBlockStoreRef()
	defer storeRef.Release()

	// Run the Ristretto controller while the generic store checks execute.
	go func() {
		_ = ctrl.Execute(ctx)
	}()

	// Await the Ristretto block store published by the controller.
	client, err := storeProm.Await(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the Ristretto store satisfies the common block store contracts.
	if err := block_store_test.TestAll(ctx, client, time.Millisecond*100); err != nil {
		t.Fatal(err.Error())
	}
}
