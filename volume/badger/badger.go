package volume_badger

import (
	"context"

	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	sbadger "github.com/aperturerobotics/hydra/store/kvtx/badger"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Badger volume controller.
const ControllerID = "hydra/volume/badger"

// Version is the version of the badger implementation.
var Version = semver.MustParse("0.0.1")

// Badger implements a BadgerDB backed volume.
type Badger = kvtx.Volume

// NewBadger builds a new Badger volume, opening the database.
func NewBadger(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Badger, error) {
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	badgerOpts, err := conf.BuildBadgerOptions()
	if err != nil {
		return nil, err
	}

	withDebugLogging := conf.GetBadgerDebug()
	store, err := sbadger.Open(
		badgerOpts.WithLogger(newBadgerLogger(le, withDebugLogging)),
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
		ControllerID,
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		false,
		store.GetDB().Close,
	)
}
