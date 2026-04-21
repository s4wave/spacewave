package store_kvtx_badger

import (
	"context"
	"testing"

	bdb "github.com/dgraph-io/badger/v4"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/sirupsen/logrus"
)

// TestBadger tests all tests on top of badger.
func TestBadger(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	o := bdb.DefaultOptions("").WithInMemory(true)
	db, err := Open(o)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.db.Close()

	ktx := store_kvtx.NewKVTx(
		kvkey,
		kvtx_vlogger.NewVLogger(le, db),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}
