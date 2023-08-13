package block_store_ristretto

import (
	"context"
	"testing"
	"time"

	block_store_test "github.com/aperturerobotics/hydra/block/store/test"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreRistretto tests the ristretto block store.
func TestBlockStoreRistretto(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	storeID := "test-store"
	ctrl := NewController(le, &Config{BlockStoreId: storeID})
	storeProm, storeRef := ctrl.AddBlockStoreRef()
	defer storeRef.Release()

	go func() {
		_ = ctrl.Execute(ctx)
	}()

	clientPtr, err := storeProm.Await(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	client := *clientPtr

	if err := block_store_test.TestAll(ctx, client, time.Millisecond*100); err != nil {
		t.Fatal(err.Error())
	}
}
