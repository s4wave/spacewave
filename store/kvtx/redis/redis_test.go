//go:build redis_test
// +build redis_test

package store_kvtx_redis

import (
	"context"
	"testing"

	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_test "github.com/aperturerobotics/hydra/store/test"
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
	vstore := kvtx_vlogger.NewVLogger(le, store)
	ktx := store_kvtx.NewKVTx(
		ctx,
		"test/redis",
		kvkey,
		vstore,
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ktx); err != nil {
		t.Fatal(err.Error())
	}
	vtx, err := vstore.NewTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	sn, err := vtx.Size()
	if err != nil {
		t.Fatal(err.Error())
	}
	vtx.Discard()
	if sn == 0 {
		t.Fatalf("expected > 0 keys but got %d from dbsize", sn)
	}
	t.Logf("finished with %d keys in db", sn)
}
