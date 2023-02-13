package mysql

import (
	"context"
	"database/sql"

	hydra_sql "github.com/aperturerobotics/hydra/sql"
)

// SqlTx implements sql.Transaction with a *Tx.
type SqlTx struct {
	*Tx
	db *sql.DB
}

// NewSqlTx constructs a new SqlTx.
func NewSqlTx(tx *Tx) (*SqlTx, error) {
	db, err := NewSqlDb(tx)
	if err != nil {
		return nil, err
	}
	return &SqlTx{Tx: tx, db: db}, nil
}

// GetReadOnly returns if the transaction is read-only.
func (r *SqlTx) GetReadOnly() bool {
	return !r.Tx.write
}

// GetDb returns the sql database.
func (r *SqlTx) GetDb(ctx context.Context) (*sql.DB, error) {
	return r.db, nil
}

// _ is a type assertion
var _ hydra_sql.Transaction = ((*SqlTx)(nil))
