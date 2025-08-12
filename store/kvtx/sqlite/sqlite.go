package store_kvtx_sqlite

import (
	"database/sql"
	"os"

	"github.com/aperturerobotics/hydra/kvtx"
)

// Store represents a SQLite-based key-value store.
type Store interface {
	kvtx.Store

	// GetDB returns the underlying SQL database connection.
	GetDB() *sql.DB

	// Close closes the database connection.
	Close() error
}

// Open opens a SQLite database store.
// The implementation will be automatically selected based on CGO availability.
func Open(path string, table string) (Store, error) {
	return open(path, table)
}

// OpenWithMode opens a SQLite database store with file mode.
// For compatibility with bolt API - mode parameter is used for file creation.
func OpenWithMode(path string, mode os.FileMode, table string) (Store, error) {
	return openWithMode(path, mode, table)
}

// NewStore constructs a new key-value store from an existing SQLite database connection.
func NewStore(db *sql.DB, table string) Store {
	return newStore(db, table)
}
