package volume_bolt

import (
	"context"

	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	sbolt "github.com/aperturerobotics/hydra/store/kvtx/bolt"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Bolt volume controller.
const ControllerID = "hydra/volume/bolt/1"

// Version is the version of the bolt implementation.
var Version = semver.MustParse("0.0.1")

// Bolt implements a BoltDB backed volume.
type Bolt = kvtx.Volume

// NewBolt builds a new Bolt volume, opening the database.
func NewBolt(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Bolt, error) {
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	store, err := sbolt.Open(
		conf.GetPath(),
		0644,
		nil,
		[]byte("hydra"),
	)
	if err != nil {
		return nil, err
	}

	var vstore skvtx.Store = store
	if conf.GetVerbose() {
		vstore = kvtx_vlogger.NewVLogger(le, vstore)
	}

	return kvtx.NewVolume(
		ctx,
		"hydra/bolt",
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
	)
}
