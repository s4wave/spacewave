package sql_rpc_client

import (
	"context"

	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
)

// Store implements the SQL store with a RPC client.
type Store struct {
	// client is the service client.
	client sql_rpc.SRPCSqlClient
}

// NewStore constructs a new SQL store.
func NewStore(client sql_rpc.SRPCSqlClient) *Store {
	return &Store{client: client}
}

// NewSqlTransaction returns a new transaction against the store.
func (s *Store) NewSqlTransaction(ctx context.Context, write bool, dsn string) (hydra_sql.SqlTransaction, error) {
	txClient, err := s.client.SqlTransaction(ctx)
	if err != nil {
		return nil, err
	}
	return InitTx(ctx, txClient, s.client.SqlTransactionRpc, write, dsn)
}

// _ is a type assertion.
var _ hydra_sql.SqlStore = (*Store)(nil)
