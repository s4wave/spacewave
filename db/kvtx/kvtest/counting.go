package kvtx_kvtest

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
)

// CountingStore counts the transactions and the operations made on a store.
// It forwards atomic commit and writer queue support, so a Volume over it
// keeps liveness as it would over the store itself.
type CountingStore struct {
	kvtx.Store
	txns, writeTxns, reads, steps, writes atomic.Uint64
}

// NewCountingStore returns a CountingStore over store.
func NewCountingStore(store kvtx.Store) *CountingStore {
	return &CountingStore{Store: store}
}

// Counts returns the operations counted so far.
func (s *CountingStore) Counts() Counts {
	return Counts{
		Txns:      s.txns.Load(),
		WriteTxns: s.writeTxns.Load(),
		Reads:     s.reads.Load(),
		Steps:     s.steps.Load(),
		Writes:    s.writes.Load(),
	}
}

// NewTransaction opens a counted transaction.
func (s *CountingStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Count the transaction, then open it.
	s.txns.Add(1)
	if write {
		s.writeTxns.Add(1)
	}
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &countingTx{Tx: tx, s: s}, nil
}

// SupportsAtomicCommit reports whether the counted store commits atomically.
func (s *CountingStore) SupportsAtomicCommit() bool {
	a, ok := s.Store.(kvtx.AtomicCommitStore)
	return ok && a.SupportsAtomicCommit()
}

// WritersWaiting reports whether a writer waits on the counted store.
func (s *CountingStore) WritersWaiting() bool {
	w, ok := s.Store.(kvtx.WriterQueueStore)
	return ok && w.WritersWaiting()
}

// countingTx counts the operations of one transaction.
type countingTx struct {
	kvtx.Tx
	s *CountingStore
}

// Get counts a point read.
func (t *countingTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	t.s.reads.Add(1)
	return t.Tx.Get(ctx, key)
}

// Exists counts a point read.
func (t *countingTx) Exists(ctx context.Context, key []byte) (bool, error) {
	t.s.reads.Add(1)
	return t.Tx.Exists(ctx, key)
}

// Set counts a write.
func (t *countingTx) Set(ctx context.Context, key, value []byte) error {
	t.s.writes.Add(1)
	return t.Tx.Set(ctx, key, value)
}

// Delete counts a write.
func (t *countingTx) Delete(ctx context.Context, key []byte) error {
	t.s.writes.Add(1)
	return t.Tx.Delete(ctx, key)
}

// ScanPrefix counts each key the scan visits.
func (t *countingTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.Tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		t.s.steps.Add(1)
		return cb(key, value)
	})
}

// ScanPrefixKeys counts each key the scan visits.
func (t *countingTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.Tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		t.s.steps.Add(1)
		return cb(key)
	})
}

// Iterate returns an iterator that counts its seeks and steps.
func (t *countingTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return &countingIterator{Iterator: t.Tx.Iterate(ctx, prefix, sort, reverse), s: t.s}
}

// countingIterator counts the seeks and steps of an iterator.
type countingIterator struct {
	kvtx.Iterator
	s *CountingStore
}

// Next counts a step.
func (i *countingIterator) Next() bool {
	i.s.steps.Add(1)
	return i.Iterator.Next()
}

// Seek counts a seek.
func (i *countingIterator) Seek(k []byte) error {
	i.s.steps.Add(1)
	return i.Iterator.Seek(k)
}

// _ is a type assertion
var (
	_ kvtx.AtomicCommitStore = (*CountingStore)(nil)
	_ kvtx.WriterQueueStore  = (*CountingStore)(nil)
)
