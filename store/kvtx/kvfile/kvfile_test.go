package store_kvtx_kvfile

import (
	"context"
	"testing"

	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/sirupsen/logrus"
)

// TestKvfile tests the kvfile volume on top of inmem.
func TestKvfile(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	ktx := store_kvtx.NewKVTx(
		ctx,
		"test/inmem",
		kvkey,
		kvtx_vlogger.NewVLogger(le, NewStore()),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}
