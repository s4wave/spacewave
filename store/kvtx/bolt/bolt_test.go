package store_kvtx_bolt

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/aperturerobotics/hydra/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/aperturerobotics/hydra/store/test"
	"github.com/sirupsen/logrus"
	bdb "go.etcd.io/bbolt"
)

// TestBolt tests all tests on top of bolt.
func TestBolt(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	dir, err := ioutil.TempDir("", "hydra-test-badger-")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)
	o := bdb.DefaultOptions(dir)
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
	).(*store_kvtx.KVTx)
	store_test.TestAll(t, ktx)
}
