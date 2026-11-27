//go:build !js

// NewSqlDb wraps a sql.SqlStore in a stdlib *stdsql.DB handle. stdlib database/sql
// pulls reflect through its convert.go Scan path, and the browser GoScript SQL
// closure never uses the high-level *stdsql.DB (it reads results through the
// driver.Rows interface in sdk/sql/query/world). Keep this file native-only
// so reflect stays out of the
// web build.
package driver

import (
	stdsql "database/sql"

	"github.com/s4wave/spacewave/db/sql"
)

// NewSqlDb opens the sql database driver with the given default dsn.
func NewSqlDb(store sql.SqlStore, dsn string) *stdsql.DB {
	return stdsql.OpenDB(NewDriver(store, dsn))
}
