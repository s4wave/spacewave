package mysql

import (
	"context"

	hydra_sql "github.com/aperturerobotics/hydra/sql"
)

// SqlTx implements sql.Transaction with a *Tx.
type SqlTx struct {
	*Tx
	db hydra_sql.SqlOps
}

// NewSqlTx constructs a new SqlTx.
func NewSqlTx(ctx context.Context, tx *Tx, dsn string) (*SqlTx, error) {
	db, err := NewSqlConn(ctx, tx, dsn)
	if err != nil {
		return nil, err
	}
	return &SqlTx{Tx: tx, db: db}, nil
}

// GetReadOnly returns if the transaction is read-only.
func (r *SqlTx) GetReadOnly() bool {
	return !r.Tx.write
}

// GetSqlOps returns the sql operations interface
func (r *SqlTx) GetSqlOps(ctx context.Context) (hydra_sql.SqlOps, error) {
	return r.db, nil
}

// _ is a type assertion
var _ hydra_sql.SqlTransaction = ((*SqlTx)(nil))
