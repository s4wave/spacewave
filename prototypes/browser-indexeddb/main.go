//go:build js

package main

import (
	"context"
	"os"

	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_kvtx_indexeddb "github.com/aperturerobotics/hydra/store/kvtx/js/indexeddb"
	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testKvStore(ctx, le)
	testKvtxStore(ctx, le)
}

func testKvStore(ctx context.Context, le *logrus.Entry) {
	st, err := store_kvtx_indexeddb.Open(ctx, "hydra/test-kv", "test-kv")
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	defer st.Close()

	vst := kvtx_vlogger.NewVLogger(le, st)
	if err := kvtx_kvtest.TestAll(ctx, vst); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	os.Stdout.WriteString("Kv test complete & successful.\n")
}

func testKvtxStore(ctx context.Context, le *logrus.Entry) {
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	st, err := store_kvtx_indexeddb.Open(ctx, "hydra/test-kvtx", "test-kvtx")
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	defer st.Close()

	ktx := store_kvtx.NewKVTx(
		kvkey,
		kvtx_vlogger.NewVLogger(le, st),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	os.Stdout.WriteString("Kvtx store test complete & successful.\n")
}
