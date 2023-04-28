package block_store_vlogger

import (
	"context"
	"testing"

	block_store_kvtx "github.com/aperturerobotics/hydra/block/store/kvtx"
	block_store_test "github.com/aperturerobotics/hydra/block/store/test"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestVLogger(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	ctx := context.Background()
	st := store_kvtx_inmem.NewStore()
	kvk, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	blockStore := block_store_kvtx.NewKVTxBlock(ctx, kvk, st, 0)
	client := NewVLoggerStore(le, blockStore)
	if err := block_store_test.TestAll(ctx, client, 0); err != nil {
		t.Fatal(err.Error())
	}
}
