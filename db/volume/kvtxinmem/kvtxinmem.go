package volume_kvtxinmem

import (
	"context"

	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/blang/semver/v4"
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
		ControllerID,
		kvkey,
		s,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		nil,
		nil,
	)
}

// _ is a type assertion
var (
	_ volume.Volume          = ((*KVTxInmem)(nil))
	_ common_kvtx.KvtxVolume = ((*KVTxInmem)(nil))
)
