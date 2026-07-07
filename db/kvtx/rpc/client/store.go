package kvtx_rpc_client

import (
	"context"
	"errors"
	"io"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
)

// Store implements the KeyValue store with a client.
type Store struct {
	// client is the service client
	client kvtx_rpc.SRPCKvtxClient
}

// NewStore constructs a new Kvtx store.
func NewStore(client kvtx_rpc.SRPCKvtxClient) *Store {
	return &Store{client: client}
}

// NewTransaction returns a new transaction against the store.
// Always call Discard() after you are done with the transaction.
// The transaction will be read-only unless write is set.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	txClient, err := s.client.KvtxTransaction(ctx)
	if err != nil {
		return nil, err
	}
	return InitTx(ctx, txClient, s.client.KvtxTransactionRpc, write)
}

// WatchPrefix streams key/value snapshots after committed store changes.
func (s *Store) WatchPrefix(ctx context.Context, prefix []byte, cb func(entries []kvtx.WatchEntry) error) error {
	if cb == nil {
		return nil
	}
	client, err := s.client.Watch(ctx, &kvtx_rpc.KvtxWatchRequest{Prefix: prefix})
	if err != nil {
		return err
	}
	defer client.Close()
	for {
		resp, err := client.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if errStr := resp.GetError(); errStr != "" {
			return errors.New(errStr)
		}
		entries := make([]kvtx.WatchEntry, 0, len(resp.GetEntries()))
		for _, entry := range resp.GetEntries() {
			entries = append(entries, kvtx.WatchEntry{
				Key:   entry.GetKey(),
				Value: entry.GetValue(),
			})
		}
		if err := cb(entries); err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ kvtx.WatchStore = ((*Store)(nil))

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
