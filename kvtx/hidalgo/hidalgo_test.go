package kvtx_hidalgo

import (
	"context"
	"testing"

	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	"github.com/hidal-go/hidalgo/kv"
	"github.com/hidal-go/hidalgo/kv/flat"
	"github.com/hidal-go/hidalgo/kv/kvtest"
	"github.com/sirupsen/logrus"
)

func TestKVTX(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	kvtest.RunTestLocal(t, func(path string) (kv.KV, error) {
		return flat.Upgrade(NewKV(ctx, store_kvtx_inmem.NewStore())), nil
	})
}
