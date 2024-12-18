//go:build !js && !wasip1
// +build !js,!wasip1

package store_kvtx_bolt

import (
	"context"
	"os"
	"path"
	"testing"

	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/sirupsen/logrus"
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

	dir, err := os.MkdirTemp("", "hydra-test-bolt-")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)

	tp := path.Join(dir, "database.boltdb")

	db, err := Open(tp, 0o644, nil, []byte("test-bucket"))
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
