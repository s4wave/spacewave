package sql

import "database/sql"

// NewSqlDb opens the sql database driver with the given default dsn.
func NewSqlDb(store SqlStore, dsn string) *sql.DB {
	return sql.OpenDB(NewDriver(store, dsn))
}
