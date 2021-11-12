package kvtx_txcache

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	sinmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestTXCache_Store(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	var underlyingStore kvtx.Store = sinmem.NewStore()
	underlyingStore = kvtx_vlogger.NewVLogger(le, underlyingStore)
	tstore := NewStore(underlyingStore)
	if err := kvtx_kvtest.TestAll(ctx, tstore); err != nil {
		t.Fatal(err.Error())
	}
}
