package volume_sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	coord_filelock "github.com/s4wave/spacewave/db/coord/filelock"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	skvtx "github.com/s4wave/spacewave/db/store/kvtx"
	sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/volume"
	kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
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
	// Open the SQLite store with the configured cache and database layout.
	pragmas := sqlite.Pragmas{
		CacheSize: conf.GetCacheSize(),
		MmapSize:  conf.GetMmapSize(),
		TempStore: int32(conf.GetTempStore()),
		PageSize:  conf.GetPageSize(),
	}
	store, err := sqlite.OpenWithPragmas(ctx, conf.GetPath(), conf.GetTable(), pragmas)
	if err != nil {
		return nil, err
	}

	// Wrap the opened store as a volume with database deletion support.
	path := conf.GetPath()
	vol, err := NewWithStore(ctx, le, conf, store, func() error { return os.Remove(path) })
	if err != nil {
		return nil, err
	}

	// Coordinate volume writes by database path and table namespace.
	vol.Coordinator = coord_filelock.NewCoordinator(
		filepath.Dir(path),
		path+"\x00"+conf.GetTable(),
		vol.Coordinator,
	)
	return vol, nil
}

// Store combines a KV transaction store with its physical SQLite pool.
type Store interface {
	skvtx.Store
	// GetDB returns the pool owned by this store.
	GetDB() *sql.DB
}

// NewWithStore takes ownership of an opened SQLite store, including on failure.
// The opener configures durability and coordinates access to the backing path.
func NewWithStore(ctx context.Context, le *logrus.Entry, conf *Config, store Store, deleteFn func() error) (*Sqlite, error) {
	// Configure the volume key namespace while taking ownership of the SQLite pool.
	db := store.GetDB()
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	// Enable transaction logging when requested by the volume configuration.
	var vstore skvtx.Store = store
	if conf.GetVerbose() {
		vstore = kvtx_vlogger.NewVLogger(le, vstore)
	}

	// Build the volume with SQLite storage statistics and pool cleanup.
	vol, err := kvtx.NewVolume(
		ctx,
		ControllerID,
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		func(ctx context.Context) (*volume.StorageStats, error) {
			// Measure the SQLite database footprint from its page count and page size.
			var pageCount, pageSize uint64
			if err := db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
				return nil, err
			}
			if err := db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
				return nil, err
			}

			// Count stored blocks in a read transaction for the volume statistics.
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
		deleteFn,
	)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return vol, nil
}

// _ is a type assertion
var (
	_ volume.Volume   = (*Sqlite)(nil)
	_ kvtx.KvtxVolume = (*Sqlite)(nil)
)
