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
	"github.com/sirupsen/logrus"
)

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
	db := store.GetDB()
	return kvtx.NewVolume(
		ctx,
		ControllerID,
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		func(ctx context.Context) (*volume.StorageStats, error) {
			var pageCount, pageSize uint64
			if err := db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
				return nil, err
			}
			if err := db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
				return nil, err
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
				TotalBytes: pageCount * pageSize,
				BlockCount: count,
			}, nil
		},
		db.Close,
		func() error { return os.Remove(path) },
	)
}

// _ is a type assertion
var (
	_ volume.Volume   = ((*Sqlite)(nil))
	_ kvtx.KvtxVolume = ((*Sqlite)(nil))
)
