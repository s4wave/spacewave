package mysql

import (
	"context"

	"github.com/dolthub/go-mysql-server/sql"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
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

// PrepareQuery describes a command on this transaction without retaining resolved
// storage handles. Its caller consumes the node before ending the transaction.
func (r *SqlTx) PrepareQuery(ctx context.Context, session sql.Session, query string) (sql.Node, error) {
	// Bind storage in the command context instead of changing the shared catalog.
	sqlCtx := r.t.engine.NewContext(context.WithValue(ctx, transactionContextKey{}, r.Tx), sql.WithSession(session))
	return r.t.engine.PrepareQuery(sqlCtx, query)
}

// GetReadOnly returns if the transaction is read-only.
func (r *SqlTx) GetReadOnly() bool {
	return !r.write
}

// GetSqlOps returns the sql operations interface.
func (r *SqlTx) GetSqlOps(ctx context.Context) (hydra_sql.SqlOps, error) {
	return r.db, nil
}

// _ is a type assertion
var _ hydra_sql.SqlTransaction = (*SqlTx)(nil)
