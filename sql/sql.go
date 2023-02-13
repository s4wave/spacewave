package sql

import (
	"context"
	"database/sql"

	"github.com/aperturerobotics/hydra/tx"
)

// SqlDB is a transactional MySQL DB.
type SqlDB interface {
	// NewTransaction starts a new SqlDB transaction.
	NewTransaction(write bool) (Transaction, error)
}

// Transaction is a SQL DB transaction.
type Transaction interface {
	// Tx is the transaction interface.
	tx.Tx
	// GetReadOnly returns if the transaction is read-only.
	GetReadOnly() bool
	// GetDb returns the sql database.
	GetDb(ctx context.Context) (*sql.DB, error)
}
