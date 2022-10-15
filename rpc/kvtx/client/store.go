package rpc_kvtx_client

import (
	"context"

	rpc_kvtx "github.com/aperturerobotics/bldr/rpc/kvtx"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Store implements the KeyValue store with a client.
type Store struct {
	// ctx is used for the calls
	ctx context.Context
	// client is the service client
	client rpc_kvtx.SRPCKvtxClient
}

// NewStore constructs a new Kvtx store.
func NewStore(ctx context.Context, client rpc_kvtx.SRPCKvtxClient) *Store {
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
	return InitTx(s.ctx, txClient, s.client.KvtxTransactionRpc)
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
