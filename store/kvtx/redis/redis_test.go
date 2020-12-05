//+build redis_test

package store_kvtx_redis

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/store/test"
	"github.com/sirupsen/logrus"
)

// TestConnURL is the URL used in tests.
var TestConnURL = "redis://localhost"

// TestRedis tests all tests on top of localhost redis.
func TestRedis(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	store, err := Connect(ctx, TestConnURL)
	if err != nil {
		t.Fatal(err.Error())
	}
	ktx := store_kvtx.NewKVTx(
		ctx,
		"test/redis",
		kvkey,
		kvtx_vlogger.NewVLogger(le, store),
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ktx); err != nil {
		t.Fatal(err.Error())
	}
}
