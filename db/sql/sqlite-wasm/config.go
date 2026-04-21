//go:build js

package sqlite_wasm

import (
	"context"
	"database/sql"
	"os"
	"strings"

	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// SqliteWasmBridgeConfig implements common.SQLiteDriverConfig for the
// sqlite-wasm RPC-based driver.
type SqliteWasmBridgeConfig struct{}

// DriverName returns the driver name for sql.Open().
func (c SqliteWasmBridgeConfig) DriverName() string {
	return driverName
}

// OpenDSN returns the DSN for sql.Open() for a given database path.
func (c SqliteWasmBridgeConfig) OpenDSN(path string) string {
	return path
}

// Description returns a human-readable description.
func (c SqliteWasmBridgeConfig) Description() string {
	return "SQLite database key-value store using sqlite.wasm via RPC"
}

// ConfigureDBPool constrains database/sql to a single underlying connection.
// The sqlite-wasm bridge models one physical sqlite database per path inside the
// Worker, so allowing database/sql to fan out would create duplicate physical
// opens for the same OPFS-backed file.
func (c SqliteWasmBridgeConfig) ConfigureDBPool(db *sql.DB) {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
}

// IsBusyError checks if the error is a SQLITE_BUSY error.
func (c SqliteWasmBridgeConfig) IsBusyError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "SQLITE_BUSY")
}

// IsNestedTxError checks if the error is a nested transaction error.
func (c SqliteWasmBridgeConfig) IsNestedTxError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "cannot start a transaction within a transaction")
}

// NewStore constructs a new key-value store from a sqlite-wasm database.
func NewStore(db *sql.DB, table string) (*common.Store[SqliteWasmBridgeConfig], error) {
	return common.NewStore(db, table, SqliteWasmBridgeConfig{})
}

// Open opens a sqlite-wasm database and wraps it in a kvtx store.
func Open(ctx context.Context, path string, table string) (*common.Store[SqliteWasmBridgeConfig], error) {
	return common.Open(ctx, path, table, SqliteWasmBridgeConfig{})
}

// OpenWithMode opens a sqlite-wasm database with mode and wraps it in a kvtx store.
func OpenWithMode(ctx context.Context, path string, mode os.FileMode, table string) (*common.Store[SqliteWasmBridgeConfig], error) {
	return common.Open(ctx, path, table, SqliteWasmBridgeConfig{})
}

// _ is a type assertion.
var _ common.SQLiteDriverConfig = SqliteWasmBridgeConfig{}
