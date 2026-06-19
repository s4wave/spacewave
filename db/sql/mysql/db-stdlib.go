//go:build !sql_lite && !js

package mysql

import (
	"context"
	"database/sql"
)

// NewSqlDb opens the sql database driver as a stdlib *sql.DB. stdlib
// database/sql pulls reflect through its convert.go Scan path, and the browser
// GoScript build never opens a high-level *sql.DB; the only caller is the native
// gorm adapter. Keep this native-only so reflect stays out of the web build.
// NOTE: dsn is used to specify arguments and is NOT the db name.
// ctx is used for the provider Resolve function.
func NewSqlDb(ctx context.Context, tx *Tx, dsn string) (*sql.DB, error) {
	conn, err := NewSqlConnector(ctx, tx, dsn)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(conn), nil
}
