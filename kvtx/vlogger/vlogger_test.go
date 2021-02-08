package kvtx_vlogger

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	sinmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestVlogger(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	var underlyingStore kvtx.Store = sinmem.NewStore()
	vstore := NewVLogger(le, underlyingStore)
	if err := kvtx_kvtest.TestAll(ctx, vstore); err != nil {
		t.Fatal(err.Error())
	}
}
