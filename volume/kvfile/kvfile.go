package volume_kvfile

import (
	"context"

	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_kvtx_kvfile "github.com/aperturerobotics/hydra/store/kvtx/kvfile"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver"
	"github.com/paralin/go-kvfile"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the KVFile volume controller.
const ControllerID = "hydra/volume/kvfile"

// Version is the version of the KVFile implementation.
var Version = semver.MustParse("0.0.1")

// KVFile implements a kvfile backed key/value tx store volume.
type KVFile = common_kvtx.Volume

// NewKVFile builds a new KVFile volume, creating the store.
func NewKVFile(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
	rdr *kvfile.Reader,
) (*KVFile, error) {
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	var s store_kvtx.Store = store_kvtx_kvfile.NewStore(rdr)
	if conf.GetVerbose() {
		s = kvtx_vlogger.NewVLogger(le, s)
	}

	return common_kvtx.NewVolume(
		ctx,
		"hydra/kvfile",
		kvkey,
		s,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		true,
	)
}
