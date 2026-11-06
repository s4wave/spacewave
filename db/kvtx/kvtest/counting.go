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
	// maxWriteOps is the most operations one write transaction made since
	// the last ResetMax.
	maxWriteOps atomic.Uint64
	// maxWriteReadBytes is the most value bytes one write transaction read
	// since the last ResetMax.
	maxWriteReadBytes atomic.Uint64
}

// NewCountingStore returns a CountingStore over store.
func NewCountingStore(store kvtx.Store) *CountingStore {
	return &CountingStore{Store: store}
}

// Counts returns the operations counted so far.
func (s *CountingStore) Counts() Counts {
	return Counts{
		Txns:              s.txns.Load(),
		WriteTxns:         s.writeTxns.Load(),
		Reads:             s.reads.Load(),
		Steps:             s.steps.Load(),
		Writes:            s.writes.Load(),
		MaxWriteOps:       s.maxWriteOps.Load(),
		MaxWriteReadBytes: s.maxWriteReadBytes.Load(),
	}
}

// ResetMax starts MaxWriteOps over, so it measures the calls made after it.
func (s *CountingStore) ResetMax() {
	s.maxWriteOps.Store(0)
	s.maxWriteReadBytes.Store(0)
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
	return &countingTx{Tx: tx, s: s, write: write}, nil
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
	// write reports a write transaction, whose operations end in MaxWriteOps.
	write bool
	// ops counts the transaction's operations.
	ops atomic.Uint64
	// readBytes counts the value bytes the transaction read.
	readBytes atomic.Uint64
	// ended reports that Commit or Discard ended the transaction.
	ended atomic.Bool
}

// count adds one operation to n and to the transaction.
func (t *countingTx) count(n *atomic.Uint64) {
	n.Add(1)
	t.ops.Add(1)
}

// Commit ends the counted transaction and commits it.
func (t *countingTx) Commit(ctx context.Context) error {
	t.end()
	return t.Tx.Commit(ctx)
}

// Discard ends the counted transaction and discards it.
func (t *countingTx) Discard() {
	t.end()
	t.Tx.Discard()
}

// end raises the store's maximums to a write transaction's counts, once.
func (t *countingTx) end() {
	if !t.write || t.ended.Swap(true) {
		return
	}
	raise(&t.s.maxWriteOps, t.ops.Load())
	raise(&t.s.maxWriteReadBytes, t.readBytes.Load())
}

// raise sets max to n when n is larger.
func raise(max *atomic.Uint64, n uint64) {
	for {
		prev := max.Load()
		if n <= prev || max.CompareAndSwap(prev, n) {
			return
		}
	}
}

// Get counts a point read.
func (t *countingTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	t.count(&t.s.reads)
	value, found, err := t.Tx.Get(ctx, key)
	t.readBytes.Add(uint64(len(value)))
	return value, found, err
}

// Exists counts a point read.
func (t *countingTx) Exists(ctx context.Context, key []byte) (bool, error) {
	t.count(&t.s.reads)
	return t.Tx.Exists(ctx, key)
}

// Set counts a write.
func (t *countingTx) Set(ctx context.Context, key, value []byte) error {
	t.count(&t.s.writes)
	return t.Tx.Set(ctx, key, value)
}

// Delete counts a write.
func (t *countingTx) Delete(ctx context.Context, key []byte) error {
	t.count(&t.s.writes)
	return t.Tx.Delete(ctx, key)
}

// ScanPrefix counts each key the scan visits.
func (t *countingTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.Tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		t.count(&t.s.steps)
		t.readBytes.Add(uint64(len(value)))
		return cb(key, value)
	})
}

// ScanPrefixKeys counts each key the scan visits.
func (t *countingTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.Tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		t.count(&t.s.steps)
		return cb(key)
	})
}

// Iterate returns an iterator that counts its seeks and steps.
func (t *countingTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return &countingIterator{Iterator: t.Tx.Iterate(ctx, prefix, sort, reverse), t: t}
}

// countingIterator counts the seeks and steps of an iterator.
type countingIterator struct {
	kvtx.Iterator
	t *countingTx
}

// Next counts a step.
func (i *countingIterator) Next() bool {
	i.t.count(&i.t.s.steps)
	return i.Iterator.Next()
}

// Value counts the bytes of the value read.
func (i *countingIterator) Value() ([]byte, error) {
	value, err := i.Iterator.Value()
	i.t.readBytes.Add(uint64(len(value)))
	return value, err
}

// ValueCopy counts the bytes of the value copied.
func (i *countingIterator) ValueCopy(dst []byte) ([]byte, error) {
	value, err := i.Iterator.ValueCopy(dst)
	i.t.readBytes.Add(uint64(len(value)))
	return value, err
}

// Seek counts a seek.
func (i *countingIterator) Seek(k []byte) error {
	i.t.count(&i.t.s.steps)
	return i.Iterator.Seek(k)
}

// _ is a type assertion
var (
	_ kvtx.AtomicCommitStore = (*CountingStore)(nil)
	_ kvtx.WriterQueueStore  = (*CountingStore)(nil)
)
