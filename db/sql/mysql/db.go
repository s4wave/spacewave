package mysql

import (
	"context"
	"database/sql/driver"

	gdriver "github.com/dolthub/go-mysql-server/driver"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
)

// NewSqlDriver constructs a sql driver from a transaction.
//
// ctx is used for the driver Resolve() function.
func NewSqlDriver(ctx context.Context, tx *Tx, driverOpts *gdriver.Options) *gdriver.Driver {
	// Borrow the store engine while preserving caller scan options.
	opts := gdriver.Options{}
	if driverOpts != nil {
		opts = *driverOpts
	}
	opts.Engine = tx.t.engine

	// The context selects this transaction in the store engine's catalog.
	provider := NewDriverProvider(ctx, tx)
	return gdriver.New(provider, &opts)
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
	// Create the SQL connector for the database transaction and DSN.
	conn, err := NewSqlConnector(ctx, tx, dsn)
	if err != nil {
		return nil, err
	}

	// Connect the SQL driver to the database transaction.
	cn, err := conn.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return cn.(SqlConn), nil
}

// _ is a type assertion
var _ SqlConn = (*gdriver.Conn)(nil)
