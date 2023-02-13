package mysql

import (
	"database/sql"

	gdriver "github.com/dolthub/go-mysql-server/driver"
)

// NewSqlDriver constructs a sql driver from a transaction.
func NewSqlDriver(tx *Tx, driverOpts *gdriver.Options) *gdriver.Driver {
	provider := NewDriverProvider(tx)
	return gdriver.New(provider, driverOpts)
}

// NewSqlDb opens the sql database driver.
func NewSqlDb(tx *Tx) (*sql.DB, error) {
	driver := NewSqlDriver(tx, &gdriver.Options{})
	// as of writing this: the dsn parsing only allows for overriding jsonAs
	var dsn string
	conn, err := driver.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(conn), nil
}
