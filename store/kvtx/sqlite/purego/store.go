package purego

import (
	"database/sql"
	"os"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/store/kvtx/sqlite/common"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
	_ "modernc.org/sqlite"
)

// PureGoConfig implements the SQLiteDriverConfig interface for pure Go SQLite driver.
type PureGoConfig struct{}

// DriverName returns the driver name for pure Go SQLite.
func (c PureGoConfig) DriverName() string {
	return "sqlite"
}

// Description returns a description for pure Go SQLite.
func (c PureGoConfig) Description() string {
	return "SQLite database key-value store using pure Go SQLite driver"
}

// IsBusyError checks if the error is a SQLITE_BUSY error for pure Go driver.
func (c PureGoConfig) IsBusyError(err error) bool {
	if sqliteErr, ok := err.(*sqlite.Error); ok {
		return sqliteErr.Code() == sqlite3.SQLITE_BUSY
	}
	return false
}

// Store is a SQLite database key-value store using pure Go SQLite driver.
type Store = common.Store[PureGoConfig]

// NewStore constructs a new key-value store from a SQLite database.
func NewStore(db *sql.DB, table string) *Store {
	return common.NewStore(db, table, PureGoConfig{})
}

// Open opens a SQLite database store using pure Go driver.
func Open(path string, table string) (*Store, error) {
	return common.Open(path, table, PureGoConfig{})
}

// OpenWithMode opens a SQLite database store with file mode.
func OpenWithMode(path string, mode os.FileMode, table string) (*Store, error) {
	return common.OpenWithMode(path, mode, table, PureGoConfig{})
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
