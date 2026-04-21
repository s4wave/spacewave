package volume_badger

import (
	"context"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	skvtx "github.com/s4wave/spacewave/db/store/kvtx"
	sbadger "github.com/s4wave/spacewave/db/store/kvtx/badger"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/volume"
	kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
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

	db := store.GetDB()
	return kvtx.NewVolume(
		ctx,
		ControllerID,
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		false,
		func(ctx context.Context) (*volume.StorageStats, error) {
			lsm, vlog := db.Size()
			if lsm < 0 || vlog < 0 {
				return nil, errors.New("badger reported negative size")
			}
			tx, err := store.NewTransaction(ctx, false)
			if err != nil {
				return nil, err
			}
			defer tx.Discard()
			count, err := tx.Size(ctx)
			if err != nil {
				return nil, err
			}
			return &volume.StorageStats{
				TotalBytes: uint64(lsm) + uint64(vlog),
				BlockCount: count,
			}, nil
		},
		db.Close,
	)
}
