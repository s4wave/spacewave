package kvtx_rpc_client

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_rpc "github.com/aperturerobotics/hydra/kvtx/rpc"
)

// Store implements the KeyValue store with a client.
type Store struct {
	// ctx is used for the calls
	ctx context.Context
	// client is the service client
	client kvtx_rpc.SRPCKvtxClient
}

// NewStore constructs a new Kvtx store.
func NewStore(ctx context.Context, client kvtx_rpc.SRPCKvtxClient) *Store {
	return &Store{ctx: ctx, client: client}
}

// NewTransaction returns a new transaction against the store.
// Always call Discard() after you are done with the transaction.
// The transaction will be read-only unless write is set.
func (s *Store) NewTransaction(write bool) (kvtx.Tx, error) {
	txClient, err := s.client.KvtxTransaction(s.ctx)
	if err != nil {
		return nil, err
	}
	return InitTx(s.ctx, txClient, s.client.KvtxTransactionRpc, write)
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
