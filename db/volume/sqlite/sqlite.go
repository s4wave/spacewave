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
	return NewSqliteWithOptions(ctx, le, conf, Options{})
}

// Options sets how NewSqliteWithOptions opens a Volume beyond its Config.
type Options struct {
	// Wrap returns the store between the database and the Volume. It takes
	// ownership of the store, including on failure; nil adds none.
	Wrap func(context.Context, Store) (Store, error)
	// WALCheckpointBytes bounds the write-ahead log, as
	// sqlite.Pragmas.WALCheckpointBytes describes; 0 keeps the store's
	// default.
	WALCheckpointBytes int64
}

// NewSqliteWithOptions builds a Sqlite volume as NewSqlite does, with opts
// applied.
func NewSqliteWithOptions(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
	opts Options,
) (*Sqlite, error) {
	// Open the SQLite store with the configured cache and database layout.
	wrap := opts.Wrap
	pragmas := sqlite.Pragmas{
		CacheSize:          conf.GetCacheSize(),
		MmapSize:           conf.GetMmapSize(),
		TempStore:          int32(conf.GetTempStore()),
		PageSize:           conf.GetPageSize(),
		WALCheckpointBytes: opts.WALCheckpointBytes,
	}
	store, err := sqlite.OpenWithPragmas(ctx, conf.GetPath(), conf.GetTable(), pragmas)
	if err != nil {
		return nil, err
	}

	// Put the wrapper between the database and the Volume.
	var vstore Store = store
	if wrap != nil {
		vstore, err = wrap(ctx, store)
		if err != nil {
			return nil, err
		}
	}

	// Wrap the opened store as a volume with database deletion support.
	path := conf.GetPath()
	vol, err := NewWithStore(ctx, le, conf, vstore, func() error { return os.Remove(path) })
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
	// Close flushes pending commits, stops the store's background work and
	// closes the pool.
	Close() error
}

// NewWithStore takes ownership of an opened SQLite store, including on failure.
// The opener configures durability and coordinates access to the backing path.
func NewWithStore(ctx context.Context, le *logrus.Entry, conf *Config, store Store, deleteFn func() error) (*Sqlite, error) {
	// Configure the volume key namespace while taking ownership of the store.
	db := store.GetDB()
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		_ = store.Close()
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
		store.Close,
		deleteFn,
	)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return vol, nil
}

// _ is a type assertion
var (
	_ volume.Volume   = (*Sqlite)(nil)
	_ kvtx.KvtxVolume = (*Sqlite)(nil)
)
