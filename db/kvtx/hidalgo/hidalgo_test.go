package kvtx_hidalgo

import (
	"testing"

	"github.com/aperturerobotics/cayley/kv"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/cayley/kv/kvtest"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestKVTX(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	kvtest.RunTestLocal(t, func(path string) (kv.KV, error) {
		return flat.Upgrade(NewKV(store_kvtx_inmem.NewStore())), nil
	}, nil)
}
