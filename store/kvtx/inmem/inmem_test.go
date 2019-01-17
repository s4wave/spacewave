package kvtx_inmem

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/store/test"
	"github.com/sirupsen/logrus"
)

// TestKVTxMQueue tests a key/value transaction message queue on top of inmem.
func TestKVTxMQueue(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	ktx := kvtx.NewKVTx(
		ctx,
		"test/inmem",
		kvkey,
		kvtx_vlogger.NewVLogger(le, NewStore()),
	).(*kvtx.KVTx)
	store_test.TestAll(t, ktx)
}
