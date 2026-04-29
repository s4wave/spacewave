package store_kvtx_sqlite

import (
	"context"
	"database/sql"
	"os"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// Pragmas re-exports common.Pragmas for callers of this package.
type Pragmas = common.Pragmas

// Store represents a SQLite-based key-value store.
type Store interface {
	kvtx.Store
	store_kvtx.Store

	// GetDB returns the underlying SQL database connection.
	GetDB() *sql.DB

	// Close closes the database connection.
	Close() error
}

// Open opens a SQLite database store.
// The implementation will be automatically selected based on CGO availability.
func Open(ctx context.Context, path string, table string) (Store, error) {
	return open(ctx, path, table)
}

// OpenWithPragmas opens a SQLite database store and applies the supplied
// tunable pragmas. The implementation is selected based on build tags.
func OpenWithPragmas(ctx context.Context, path string, table string, pragmas Pragmas) (Store, error) {
	return openWithPragmas(ctx, path, table, pragmas)
}

// OpenWithMode opens a SQLite database store with file mode.
func OpenWithMode(ctx context.Context, path string, mode os.FileMode, table string) (Store, error) {
	return openWithMode(ctx, path, mode, table)
}

// NewStore constructs a new key-value store from an existing SQLite database connection.
func NewStore(db *sql.DB, table string) (Store, error) {
	return newStore(db, table)
}
