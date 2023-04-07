package kvtx_kvfile

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/paralin/go-kvfile"
)

// ErrCannotWrite is returned if we try to write to a kvfile.
var ErrCannotWrite = errors.New("kvfile: read-only: cannot perform write tx")

// KvfileTx implements a read-only transaction against a kvfile.
type KvfileTx struct {
	rdr       *kvfile.Reader
	write     bool
	discarded atomic.Bool
}

// NewTransaction returns a new transaction against the store.
// The transaction will be read-only regardless of write.
func NewTransaction(rdr *kvfile.Reader, write bool) *KvfileTx {
	return &KvfileTx{rdr: rdr, write: write}
}

// GetKvfileReader returns the inner kvfile reader.
func (s *KvfileTx) GetKvfileReader() *kvfile.Reader {
	return s.rdr
}

// Size returns the number of keys in the store.
func (s *KvfileTx) Size() (uint64, error) {
	return s.rdr.Size(), nil
}

// Get returns values for a key.
func (s *KvfileTx) Get(key []byte) (data []byte, found bool, err error) {
	return s.rdr.Get(key)
}

// Exists checks if a key exists.
func (s *KvfileTx) Exists(key []byte) (bool, error) {
	return s.rdr.Exists(key)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (s *KvfileTx) Set(key, value []byte) error {
	if !s.write {
		return kvtx.ErrNotWrite
	}
	if s.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	return ErrCannotWrite
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (s *KvfileTx) Delete(key []byte) error {
	if !s.write {
		return kvtx.ErrNotWrite
	}
	if s.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	return ErrCannotWrite
}

// ScanPrefix iterates over keys with a prefix.
func (s *KvfileTx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	if s.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	return s.rdr.ScanPrefix(prefix, cb)
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (s *KvfileTx) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	if s.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	return s.rdr.ScanPrefixKeys(prefix, cb)
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse has no effect.
// Must call Next() or Seek() before valid.
// Some implementations return BlockIterator.
func (s *KvfileTx) Iterate(prefix []byte, sort, reverse bool) kvtx.Iterator {
	if !sort {
		reverse = false
	}
	return NewIterator(s.rdr, prefix, reverse)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (s *KvfileTx) Commit(ctx context.Context) error {
	if !s.write {
		return kvtx.ErrNotWrite
	}
	if s.discarded.Swap(true) {
		return kvtx.ErrDiscarded
	}
	return nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (s *KvfileTx) Discard() {
	s.discarded.Store(true)
}

// _ is a type assertion
var _ kvtx.Tx = ((*KvfileTx)(nil))
