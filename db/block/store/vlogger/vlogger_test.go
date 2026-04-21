package block_store_vlogger

import (
	"context"
	"testing"

	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	block_store_test "github.com/s4wave/spacewave/db/block/store/test"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
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
	blockStoreOps := block_store_kvtx.NewKVTxBlock(kvk, st, 0, true)
	blockStore := block_store.NewStore("test/store", blockStoreOps)
	client := NewVLoggerStore(le, blockStore)
	if err := block_store_test.TestAll(ctx, client, 0); err != nil {
		t.Fatal(err.Error())
	}
}
