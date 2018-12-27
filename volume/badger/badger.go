package volume_badger

import (
	"context"

	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	sbadger "github.com/aperturerobotics/hydra/store/kvtx/badger"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver"
)

// ControllerID identifies the Badger volume controller.
const ControllerID = "hydra/volume/badger/1"

// Version is the version of the badger implementation.
var Version = semver.MustParse("0.0.1")

// Badger implements a BadgerDB backed volume.
type Badger = kvtx.Volume

// NewBadger builds a new Badger volume, opening the database.
func NewBadger(
	ctx context.Context,
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

	store, err := sbadger.Open(*badgerOpts)
	if err != nil {
		return nil, err
	}

	return kvtx.NewVolume(
		ctx,
		"hydra/badger",
		kvkey,
		store,
		conf.GetNoGenerateKey(),
	)
}
