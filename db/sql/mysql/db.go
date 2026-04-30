//go:build !sql_lite

package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"

	gdriver "github.com/dolthub/go-mysql-server/driver"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
)

// NewSqlDriver constructs a sql driver from a transaction.
//
// ctx is used for the driver Resolve() function.
func NewSqlDriver(ctx context.Context, tx *Tx, driverOpts *gdriver.Options) *gdriver.Driver {
	provider := NewDriverProvider(ctx, tx)
	return gdriver.New(provider, driverOpts)
}

// NewSqlConnector constructs a new sql conn from a transaction.
// NOTE: dsn is used to specify arguments and is NOT the db name.
// ctx is used for the driver Resolve() function.
func NewSqlConnector(ctx context.Context, tx *Tx, dsn string) (driver.Connector, error) {
	driver := NewSqlDriver(ctx, tx, &gdriver.Options{})
	return driver.OpenConnector(dsn)
}

// SqlConn is the set of interfaces the mysql driver conn implements.
type SqlConn interface {
	driver.Conn
	hydra_sql.SqlOps
}

// NewSqlConn creates a sql conn from a transaction and dsn.
// NOTE: dsn is used to specify arguments and is NOT the db name.
func NewSqlConn(ctx context.Context, tx *Tx, dsn string) (SqlConn, error) {
	conn, err := NewSqlConnector(ctx, tx, dsn)
	if err != nil {
		return nil, err
	}
	cn, err := conn.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return cn.(SqlConn), nil
}

// NewSqlDb opens the sql database driver.
// NOTE: dsn is used to specify arguments and is NOT the db name.
// ctx is used for the provider Resolve function.
func NewSqlDb(ctx context.Context, tx *Tx, dsn string) (*sql.DB, error) {
	conn, err := NewSqlConnector(ctx, tx, dsn)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(conn), nil
}

// _ is a type assertion
var _ SqlConn = (*gdriver.Conn)(nil)
