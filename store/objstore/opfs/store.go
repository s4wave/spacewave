//go:build js

// Package store_objstore_opfs implements a kvtx.Store for world objects
// backed by OPFS with per-file WebLock coordination.
//
// Read transactions acquire a shared WebLock and read from OPFS. Write
// transactions buffer mutations in memory and apply them at commit time
// with an exclusive WebLock and per-file locks for each file mutation.
package store_objstore_opfs

import (
	"context"
	"encoding/hex"
	"syscall/js"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_txcache "github.com/aperturerobotics/hydra/kvtx/txcache"
	"github.com/aperturerobotics/hydra/opfs"
	store_kvtx_opfs "github.com/aperturerobotics/hydra/store/kvtx/js/opfs"
)

// Store is an OPFS-backed object store with per-file write locking.
// Uses a shared/exclusive WebLock for transaction-level ACID and per-file
// locks at commit time for individual mutations.
type Store struct {
	// kvStore handles the existing kvtx read path (shared WebLock,
	// hex-encoded keys, 2-char shard directories, scan/iterate).
	kvStore *store_kvtx_opfs.Store
	// root is the OPFS directory handle for this store's data.
	root js.Value
	// lockName is the WebLock key for transaction coordination
	// (e.g. "vol-id|objstore").
	lockName string
	// lockPrefix is the WebLock name prefix for per-file locks
	// (e.g. "vol-id/obj").
	lockPrefix string
}

// NewStore constructs a new object store.
// root is the OPFS directory handle. lockName is the WebLock key for
// transactions (e.g. "vol-id|objstore"). lockPrefix is the per-file
// WebLock name prefix (e.g. "vol-id/obj").
func NewStore(root js.Value, lockName, lockPrefix string) *Store {
	return &Store{
		kvStore:    store_kvtx_opfs.NewStore(root, lockName),
		root:       root,
		lockName:   lockName,
		lockPrefix: lockPrefix,
	}
}

// NewTransaction returns a new transaction against the store.
// Read transactions use a shared WebLock with the existing kvtx read path.
// Write transactions buffer mutations in memory and commit with an
// exclusive WebLock and per-file locking.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		return s.kvStore.NewTransaction(ctx, false)
	}

	// Read tx for the txcache to read against during the write phase.
	readTx, err := s.kvStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}

	return kvtx_txcache.NewTxWithCbs(
		readTx,        // underlying read ops (shared WebLock)
		true,          // write
		readTx.Discard, // closeReadTx: releases the shared lock
		func() (kvtx.Tx, error) {
			return newWriteTx(s)
		},
		true, // commitWriteTx
	)
}

// Execute is a no-op for OPFS object stores.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// encodeKey hex-encodes a key for use as an OPFS filename.
func encodeKey(key []byte) string {
	return hex.EncodeToString(key)
}

// shardPrefix returns the 2-char shard directory name for an encoded key.
func shardPrefix(encoded string) string {
	if len(encoded) < 2 {
		return "00"
	}
	return encoded[:2]
}

// getShardDir returns the shard directory, creating it if create is true.
func getShardDir(root js.Value, shard string, create bool) (js.Value, error) {
	return opfs.GetDirectory(root, shard, create)
}

// _ is a type assertion.
var _ kvtx.Store = (*Store)(nil)
