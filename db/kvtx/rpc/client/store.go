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
	return s.WatchPrefixBounded(ctx, prefix, kvtx.WatchLimits{}, cb)
}

// WatchPrefixBounded streams key/value snapshots after committed store changes
// while each snapshot stays within limits.
// Returns ErrWatchLimit without any callback when a snapshot exceeds limits.
func (s *Store) WatchPrefixBounded(ctx context.Context, prefix []byte, limits kvtx.WatchLimits, cb func(entries []kvtx.WatchEntry) error) error {
	// Require a snapshot consumer before opening the remote watch.
	if cb == nil {
		return nil
	}

	// Open a remote watch with the requested snapshot limits.
	client, err := s.client.Watch(ctx, &kvtx_rpc.KvtxWatchRequest{
		Prefix:     prefix,
		MaxRecords: limits.MaxRecords,
		MaxBytes:   limits.MaxBytes,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	// Deliver bounded key snapshots until the remote watch ends.
	for {
		// Read the next committed key snapshot from the remote store.
		resp, err := client.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Preserve remote failures and snapshot limit errors.
		if errStr := resp.GetError(); errStr != "" {
			if resp.GetLimitExceeded() {
				return kvtx.ErrWatchLimit
			}
			return errors.New(errStr)
		}

		// Convert the remote snapshot and deliver it to the consumer.
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
var _ kvtx.WatchStore = (*Store)(nil)

// _ is a type assertion
var _ kvtx.BoundedWatchStore = (*Store)(nil)

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
