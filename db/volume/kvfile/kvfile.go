package volume_kvfile

import (
	"context"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/blang/semver/v4"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_kvfile "github.com/s4wave/spacewave/db/store/kvtx/kvfile"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the KVFile volume controller.
const ControllerID = "hydra/volume/kvfile"

// Version is the version of the KVFile implementation.
var Version = semver.MustParse("0.0.1")

// ctrlDescrip is the controller description
var ctrlDescrip = "kvfile read-only volume"

// KVFile implements a kvfile backed key/value tx store volume.
type KVFile = common_kvtx.Volume

// NewKVFile builds a new KVFile volume, creating the store.
func NewKVFile(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
	rdr *kvfile.Reader,
	closeFn func() error,
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
		ControllerID,
		kvkey,
		s,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		true,
		nil,
		closeFn,
	)
}
