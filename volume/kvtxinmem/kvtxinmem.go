package volume_kvtxinmem

import (
	"context"

	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	sinmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the KVTxInmem volume controller.
const ControllerID = "hydra/volume/kvtxinmem"

// Version is the version of the KVTxInmem implementation.
var Version = semver.MustParse("0.0.1")

// KVTxInmem implements a in-memory key/value tx store volume.
type KVTxInmem = common_kvtx.Volume

// NewKVTxInmem builds a new KVTxInmem volume, creating the store.
func NewKVTxInmem(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*KVTxInmem, error) {
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	var s store_kvtx.Store = sinmem.NewStore()
	if conf.GetVerbose() {
		s = kvtx_vlogger.NewVLogger(le, s)
	}

	return common_kvtx.NewVolume(
		ctx,
		"hydra/kvtxinmem",
		kvkey,
		s,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
	)
}
