//go:build !sql_lite

package mysql

import (
	"context"

	"github.com/s4wave/spacewave/db/sql"
)

// NewTransaction returns a new SqlDB transaction.
func (t *Mysql) NewSqlTransaction(ctx context.Context, write bool, dsn string) (sql.SqlTransaction, error) {
	mtx, err := t.NewMysqlTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	stx, err := NewSqlTx(ctx, mtx, dsn)
	if err != nil {
		mtx.Discard()
		return nil, err
	}
	return stx, nil
}

// _ is a type assertion
var _ sql.SqlStore = ((*Mysql)(nil))
