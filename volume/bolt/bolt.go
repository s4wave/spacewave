//go:build !js && !wasip1
// +build !js,!wasip1

package volume_bolt

import (
	"context"

	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	sbolt "github.com/aperturerobotics/hydra/store/kvtx/bolt"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/volume"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
	bdb "go.etcd.io/bbolt"
)

// ControllerID identifies the Bolt volume controller.
const ControllerID = "hydra/volume/bolt"

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

	// set defaults for performance with single writer on db
	bdbOpts := &bdb.Options{
		Timeout:        0,
		NoFreelistSync: !conf.GetFreelistSync(),
		NoGrowSync:     false,
		FreelistType:   bdb.FreelistMapType,
		NoSync:         !conf.GetSync(),
	}

	store, err := sbolt.Open(
		conf.GetPath(),
		0o644,
		bdbOpts,
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
		ControllerID,
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		store.GetDB().Close,
	)
}

// _ is a type assertion
var (
	_ volume.Volume          = ((*Bolt)(nil))
	_ common_kvtx.KvtxVolume = ((*Bolt)(nil))
)
