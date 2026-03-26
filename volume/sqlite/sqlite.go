package volume_sqlite

import (
	"context"
	"os"

	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	sqlite "github.com/aperturerobotics/hydra/store/kvtx/sqlite"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/volume"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Sqlite volume controller.
const ControllerID = "hydra/volume/sqlite"

// Version is the version of the sqlite implementation.
var Version = semver.MustParse("0.0.1")

// Sqlite implements a SqliteDB backed volume.
type Sqlite = kvtx.Volume

// NewSqlite builds a new Sqlite volume, opening the database.
func NewSqlite(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Sqlite, error) {
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	store, err := sqlite.Open(ctx, conf.GetPath(), conf.GetTable())
	if err != nil {
		return nil, err
	}

	var vstore skvtx.Store = store
	if conf.GetVerbose() {
		vstore = kvtx_vlogger.NewVLogger(le, vstore)
	}

	path := conf.GetPath()
	return kvtx.NewVolume(
		ctx,
		ControllerID,
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		store.GetDB().Close,
		func() error { return os.Remove(path) },
	)
}

// _ is a type assertion
var (
	_ volume.Volume   = ((*Sqlite)(nil))
	_ kvtx.KvtxVolume = ((*Sqlite)(nil))
)
