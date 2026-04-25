//go:build js && wasm

// Package store implements a kvtx.Store backed by OPFS flat files.
//
// Each key-value pair is stored as a file in a sharded directory structure.
// Keys are base58-encoded as filenames, sharded by the first character.
// Values are the raw file contents.
package store

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
	opfs "github.com/s4wave/spacewave/prototypes/opfs/go-opfs"
)

// Store is an OPFS-backed key-value store implementing kvtx.Store.
type Store struct {
	root  *opfs.DirectoryHandle
	data  *opfs.DirectoryHandle
	cache *handleCache

	mu    sync.Mutex
	tally uint64
}

// NewStore creates a new OPFS store rooted at the given directory.
// It creates a "data" subdirectory for key-value storage.
func NewStore(root *opfs.DirectoryHandle) (*Store, error) {
	data, err := root.GetDirectoryHandle("data", true)
	if err != nil {
		return nil, err
	}
	s := &Store{
		root:  root,
		data:  data,
		cache: newHandleCache(),
	}
	return s, nil
}

// Open creates a new OPFS store in a subdirectory of the OPFS root.
func Open(name string) (*Store, error) {
	root, err := opfs.GetRootDirectory()
	if err != nil {
		return nil, err
	}
	dir, err := root.GetDirectoryHandle(name, true)
	if err != nil {
		return nil, err
	}
	return NewStore(dir)
}

// NewTransaction returns a new transaction against the store.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	readTx := newTx(s, false)
	if !write {
		return readTx, nil
	}
	return kvtx_txcache.NewTxWithCbs(readTx, true, readTx.Discard, func() (kvtx.Tx, error) {
		return newTx(s, true), nil
	}, true)
}

// GetStorageTally returns the current storage tally in bytes.
func (s *Store) GetStorageTally() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tally
}

// Close releases resources held by the store.
func (s *Store) Close() {
	s.cache.closeAll()
}

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
