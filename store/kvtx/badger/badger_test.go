package store_kvtx_badger

import (
	"context"
	"testing"

	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_test "github.com/aperturerobotics/hydra/store/test"
	bdb "github.com/dgraph-io/badger/v2"
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
	bdb.DefaultOptions("")
	o := bdb.DefaultOptions("").WithInMemory(true)
	db, err := Open(o)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.db.Close()

	ktx := store_kvtx.NewKVTx(
		ctx,
		"test/badger",
		kvkey,
		kvtx_vlogger.NewVLogger(le, db),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ktx); err != nil {
		t.Fatal(err.Error())
	}
}
