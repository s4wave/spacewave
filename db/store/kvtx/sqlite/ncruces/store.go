//go:build !js

package sqlite_ncruces

// This technically works on GOOS=js GOARCH=wasm but has test failures / unpredictable behavior.
// See: https://github.com/ncruces/go-sqlite3/pull/369
// See: https://github.com/ncruces/go-sqlite3/issues/370
// Worth revisiting in future if the "found bad pointer in Go heap" issue is diagnosed.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	sqlite "github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/memdb"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// SqliteWasmConfig implements the SQLiteDriverConfig interface for pure Go SQLite driver.
// This uses github.com/ncruces/go-sqlite3 which is sqlite -> wasm -> go.
type SqliteWasmConfig struct{}

// DriverName returns the driver name for pure Go SQLite.
func (c SqliteWasmConfig) DriverName() string {
	return "sqlite3"
}

// OpenDSN returns the DSN to use with sql.Open(). It selects incremental
// auto-vacuum ahead of WAL mode: a fresh file takes auto-vacuum only before
// its header is written, which selecting WAL mode does, and an existing file
// keeps its mode until a VACUUM.
func (c SqliteWasmConfig) OpenDSN(path string) string {
	return "file:" + filepath.ToSlash(path) +
		"?_pragma=auto_vacuum(INCREMENTAL)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
}

// OpenReadOnlyDSN returns the DSN that opens the database at path read-only.
func (c SqliteWasmConfig) OpenReadOnlyDSN(path string) string {
	return "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=busy_timeout(5000)"
}

// Description returns a description for pure Go SQLite.
func (c SqliteWasmConfig) Description() string {
	return "SQLite database key-value store using SQLite to wasm to pure Go driver"
}

// IsBusyError checks if the error is a SQLITE_BUSY error for pure Go driver.
func (c SqliteWasmConfig) IsBusyError(err error) bool {
	if sqliteErr, ok := err.(*sqlite.Error); ok {
		return sqliteErr.Code() == sqlite.BUSY
	}
	return false
}

// SyncWAL flushes the database's WAL on the local file system.
func (c SqliteWasmConfig) SyncWAL(path string) error {
	return common.SyncLocalWAL(path)
}

// Store is a SQLite database key-value store using pure Go SQLite driver.
type Store = common.Store[SqliteWasmConfig]

// NewStore constructs a new key-value store from a SQLite database.
func NewStore(db *sql.DB, table string) (*Store, error) {
	return common.NewStore(db, table, SqliteWasmConfig{})
}

// Open opens a SQLite database store using pure Go driver.
func Open(ctx context.Context, path string, table string) (*Store, error) {
	return common.Open(ctx, path, table, SqliteWasmConfig{})
}

// OpenWithPragmas opens a SQLite database store using pure Go driver and
// applies the supplied tunable pragmas.
func OpenWithPragmas(ctx context.Context, path string, table string, pragmas common.Pragmas) (*Store, error) {
	return common.OpenWithPragmas(ctx, path, table, pragmas, SqliteWasmConfig{})
}

// OpenWithMode opens a SQLite database store with file mode.
func OpenWithMode(ctx context.Context, path string, mode os.FileMode, table string) (*Store, error) {
	return common.OpenWithMode(ctx, path, mode, table, SqliteWasmConfig{})
}

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
