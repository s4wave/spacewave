//go:build !js

// NewSqlDb wraps a SqlStore in a stdlib *sql.DB handle. stdlib database/sql
// pulls reflect through its convert.go Scan path, and the browser GoScript SQL
// closure never uses the high-level *sql.DB (it reads results through the
// driver.Rows interface in sdk/sql/query/world); the only caller is the native
// db/sql/mock helper. Keep this file native-only so reflect stays out of the
// web build.
package sql

import "database/sql"

// NewSqlDb opens the sql database driver with the given default dsn.
func NewSqlDb(store SqlStore, dsn string) *sql.DB {
	return sql.OpenDB(NewDriver(store, dsn))
}
