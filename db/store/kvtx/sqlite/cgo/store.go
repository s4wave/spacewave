//go:build cgo

package cgo

import (
	"context"
	"database/sql"
	"os"

	"github.com/mattn/go-sqlite3"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// CGOConfig implements the SQLiteDriverConfig interface for CGO SQLite driver.
type CGOConfig struct{}

// DriverName returns the driver name for CGO SQLite.
func (c CGOConfig) DriverName() string {
	return "sqlite3"
}

// OpenDSN returns the DSN to use with sql.Open().
func (c CGOConfig) OpenDSN(path string) string {
	return path + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"
}

// OpenCreateDSN returns the DSN that opens the database at path without
// selecting a journal mode, to shape a fresh file before WAL mode.
func (c CGOConfig) OpenCreateDSN(path string) string {
	return path + "?_busy_timeout=5000"
}

// OpenReadOnlyDSN returns the DSN that opens the database at path read-only.
func (c CGOConfig) OpenReadOnlyDSN(path string) string {
	return "file:" + path + "?mode=ro&_busy_timeout=5000"
}

// Description returns a description for CGO SQLite.
func (c CGOConfig) Description() string {
	return "SQLite database key-value store using CGO SQLite driver"
}

// IsBusyError checks if the error is a SQLITE_BUSY error for CGO driver.
func (c CGOConfig) IsBusyError(err error) bool {
	sqliteErr, ok := err.(sqlite3.Error)
	return ok && sqliteErr.Code == sqlite3.ErrBusy
}

// SyncWAL flushes the database's WAL on the local file system.
func (c CGOConfig) SyncWAL(path string) error {
	return common.SyncLocalWAL(path)
}

// Store is a SQLite database key-value store using CGO SQLite driver.
type Store = common.Store[CGOConfig]

// NewStore constructs a new key-value store from a SQLite database.
func NewStore(db *sql.DB, table string) (*Store, error) {
	return common.NewStore(db, table, CGOConfig{})
}

// Open opens a SQLite database store using CGO driver.
func Open(ctx context.Context, path string, table string) (*Store, error) {
	return common.Open(ctx, path, table, CGOConfig{})
}

// OpenWithPragmas opens a SQLite database store using CGO driver and applies
// the supplied tunable pragmas.
func OpenWithPragmas(ctx context.Context, path string, table string, pragmas common.Pragmas) (*Store, error) {
	return common.OpenWithPragmas(ctx, path, table, pragmas, CGOConfig{})
}

// OpenWithMode opens a SQLite database store with file mode.
func OpenWithMode(ctx context.Context, path string, mode os.FileMode, table string) (*Store, error) {
	return common.OpenWithMode(ctx, path, mode, table, CGOConfig{})
}

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
