package sql

import (
	"context"
	"database/sql/driver"

	"github.com/s4wave/spacewave/db/tx"
)

// SqlStore is a transactional MySQL store.
type SqlStore interface {
	// NewSqlTransaction starts a new SqlStore transaction.
	//
	// If !write, the transaction should be read-only.
	// dsn is the default database name for the transaction.
	NewSqlTransaction(
		ctx context.Context,
		write bool,
		dsn string,
	) (SqlTransaction, error)
}

// SqlTransaction is a SQL DB transaction.
type SqlTransaction interface {
	// Tx is the transaction interface.
	tx.Tx
	// GetReadOnly returns if the transaction is read-only.
	GetReadOnly() bool
	// GetSqlOps returns the sql operations interface.
	// see the comments in the stdlib sql/driver package for more information.
	GetSqlOps(ctx context.Context) (SqlOps, error)
}

// SqlOps are operations on the SQL DB transaction.
type SqlOps interface {
	driver.Execer //nolint:staticcheck
	driver.ExecerContext

	driver.Queryer //nolint:staticcheck
	driver.QueryerContext
}
