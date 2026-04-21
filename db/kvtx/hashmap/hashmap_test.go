package hashmap

import (
	"context"
	"testing"

	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	"github.com/sirupsen/logrus"
)

func TestHashMapKVTX(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	m := NewHashmap[[]byte]()
	store := NewHashmapKvtx(m)
	store = kvtx_vlogger.NewVLogger(le, store)
	if err := kvtx_kvtest.TestAll(ctx, store); err != nil {
		t.Fatal(err.Error())
	}
}
