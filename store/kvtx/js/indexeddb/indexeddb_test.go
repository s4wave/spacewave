//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"context"
	"testing"

	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/sirupsen/logrus"
)

// TestIndexedDB tests all tests on top of indexeddb.
func TestIndexedDB(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	st, err := Open(ctx, "hydra/test-db", "test-store")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer st.db.Close()

	ktx := store_kvtx.NewKVTx(
		kvkey,
		kvtx_vlogger.NewVLogger(le, st),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}
