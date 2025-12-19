//go:build cgo

package cgo

import (
	"context"
	"database/sql"
	"os"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/store/kvtx/sqlite/common"
	"github.com/mattn/go-sqlite3"
	_ "github.com/mattn/go-sqlite3"
)

// CGOConfig implements the SQLiteDriverConfig interface for CGO SQLite driver.
type CGOConfig struct{}

// DriverName returns the driver name for CGO SQLite.
func (c CGOConfig) DriverName() string {
	return "sqlite3"
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

// IsNestedTxError checks if the error is a nested transaction error for CGO driver.
// This occurs when BeginTx is called on a connection that already has an active transaction.
func (c CGOConfig) IsNestedTxError(err error) bool {
	sqliteErr, ok := err.(sqlite3.Error)
	// ErrError is SQLITE_ERROR (1)
	return ok && sqliteErr.Code == sqlite3.ErrError
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

// OpenWithMode opens a SQLite database store with file mode.
func OpenWithMode(ctx context.Context, path string, mode os.FileMode, table string) (*Store, error) {
	return common.OpenWithMode(ctx, path, mode, table, CGOConfig{})
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
