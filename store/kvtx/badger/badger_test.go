package kvtx_badger

import (
	"context"
	"os"
	"testing"

	"github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/store/test"
	bdb "github.com/dgraph-io/badger"
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
	o := bdb.DefaultOptions
	o.Dir = "./test-badger-db"
	o.ValueDir = o.Dir
	defer os.RemoveAll(o.Dir)

	db, err := Open(o)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.db.Close()

	ktx := kvtx.NewKVTx(ctx, "test/badger", kvkey, kvtx_vlogger.NewVLogger(le, db)).(*kvtx.KVTx)
	store_test.TestAll(t, ktx)
}
