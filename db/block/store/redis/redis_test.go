//go:build test_redis

package block_store_redis

import (
	"context"
	"testing"
	"time"

	block_store_test "github.com/s4wave/spacewave/db/block/store/test"
	store_kvtx_redis "github.com/s4wave/spacewave/db/store/kvtx/redis"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreRedis tests the redis block store.
func TestBlockStoreRedis(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	storeID := "test-store"
	ctrl := NewController(le, &Config{
		BlockStoreId: storeID,
		Client: &store_kvtx_redis.ClientConfig{
			Url: "redis://localhost",
		},
	})
	storeProm, storeRef := ctrl.AddBlockStoreRef()
	defer storeRef.Release()

	go ctrl.Execute(ctx)

	clientPtr, err := storeProm.Await(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	client := *clientPtr

	if err := block_store_test.TestAll(ctx, client, time.Millisecond*100); err != nil {
		t.Fatal(err.Error())
	}
}
