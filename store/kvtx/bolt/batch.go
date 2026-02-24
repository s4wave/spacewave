//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	bdb "go.etcd.io/bbolt"
	bdberrors "go.etcd.io/bbolt/errors"
)

// flushInterval is how long unflushed writes can sit before a periodic commit.
const flushInterval = 100 * time.Millisecond

// BatchStore wraps a bolt Store and coalesces multiple write transactions
// into a single BoltDB write transaction, committing every batchSize writes.
//
// This dramatically reduces fsync overhead for bulk write workloads like
// git imports where each block results in a separate PutBlock call.
//
// Read transactions pass through to the underlying BoltDB directly.
// Write transactions share a single BoltDB write tx; the real commit
// happens when the batch is full, Flush is called, or the periodic
// flush timer fires (100ms after the first unflushed write).
type BatchStore struct {
	store     *Store
	batchSize int

	mu          sync.Mutex
	writeTx     *bdb.Tx
	pending     int
	flushCancel chan struct{} // closed to cancel the periodic flush goroutine

	stats BatchStats
}

// BatchStats tracks batch store performance.
type BatchStats struct {
	Writes  atomic.Int64
	Commits atomic.Int64
}

// NewBatchStore constructs a BatchStore wrapping the given bolt Store.
// batchSize controls how many write transactions are coalesced into one
// BoltDB write transaction. A value of 0 or 1 disables batching.
func NewBatchStore(store *Store, batchSize int) *BatchStore {
	if batchSize < 1 {
		batchSize = 1
	}
	return &BatchStore{store: store, batchSize: batchSize}
}

// GetStats returns the batch stats.
func (b *BatchStore) GetStats() (writes, commits int64) {
	return b.stats.Writes.Load(), b.stats.Commits.Load()
}

// NewTransaction returns a new transaction.
// Write transactions are batched; read transactions pass through.
func (b *BatchStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		// Flush pending writes so read transactions see the latest state.
		b.mu.Lock()
		if err := b.flush(); err != nil {
			b.mu.Unlock()
			return nil, err
		}
		b.mu.Unlock()
		return b.store.NewTransaction(ctx, false)
	}

	b.mu.Lock()
	if b.writeTx == nil {
		var err error
		b.writeTx, err = b.store.db.Begin(true)
		if err != nil {
			b.mu.Unlock()
			return nil, err
		}
		b.pending = 0
	}
	// mu is held until batchTx.Commit() or batchTx.Discard()
	return &batchTx{batch: b, bucket: b.store.bucket}, nil
}

// Flush commits any pending batched writes.
func (b *BatchStore) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flush()
}

// flush commits the current write tx. Must hold mu.
func (b *BatchStore) flush() error {
	if b.writeTx == nil {
		return nil
	}
	if b.flushCancel != nil {
		close(b.flushCancel)
		b.flushCancel = nil
	}
	err := b.writeTx.Commit()
	b.writeTx = nil
	b.pending = 0
	b.stats.Commits.Add(1)
	return err
}

// timerFlush is spawned as a goroutine when the first write in a batch
// is committed. It waits flushInterval then commits pending writes,
// bounding worst-case data loss to flushInterval.
func (b *BatchStore) timerFlush(cancel chan struct{}) {
	timer := time.NewTimer(flushInterval)
	defer timer.Stop()
	select {
	case <-timer.C:
		b.mu.Lock()
		if b.flushCancel == cancel {
			_ = b.flush()
		}
		b.mu.Unlock()
	case <-cancel:
	}
}

// Execute executes the store (no-op, satisfies controller interface).
func (b *BatchStore) Execute(ctx context.Context) error {
	return nil
}

// batchTx is a virtual write transaction within a batch.
type batchTx struct {
	batch  *BatchStore
	bucket []byte
	done   bool
}

// getBucket returns or creates the bucket within the shared write tx.
func (t *batchTx) getBucket() (*bdb.Bucket, error) {
	return t.batch.writeTx.CreateBucketIfNotExists(t.bucket)
}

// Get returns values for a key.
func (t *batchTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	bkt, err := t.getBucket()
	if err != nil {
		return nil, false, err
	}
	value := bkt.Get(key)
	if value == nil {
		return nil, false, nil
	}
	// Value is only valid during tx, clone it.
	out := make([]byte, len(value))
	copy(out, value)
	return out, true, nil
}

// Size returns the number of keys in the store.
func (t *batchTx) Size(ctx context.Context) (uint64, error) {
	bkt, err := t.getBucket()
	if err != nil {
		return 0, err
	}
	return uint64(bkt.Stats().KeyN), nil
}

// Set sets the value of a key.
func (t *batchTx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	bkt, err := t.getBucket()
	if err != nil {
		return err
	}
	t.batch.stats.Writes.Add(1)
	return bkt.Put(key, value)
}

// Exists checks if a key exists.
func (t *batchTx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	bkt, err := t.getBucket()
	if err != nil {
		if err == bdberrors.ErrBucketNotFound {
			return false, nil
		}
		return false, err
	}
	return bkt.Get(key) != nil, nil
}

// Delete deletes a key.
func (t *batchTx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	bkt, err := t.getBucket()
	if err != nil {
		return err
	}
	return bkt.Delete(key)
}

// ScanPrefix iterates over keys with a prefix.
func (t *batchTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	bkt, err := t.getBucket()
	if err != nil {
		return err
	}
	return bkt.ForEach(func(k, v []byte) error {
		if len(prefix) != 0 {
			if len(k) < len(prefix) {
				return nil
			}
			for i := range prefix {
				if k[i] != prefix[i] {
					return nil
				}
			}
		}
		return cb(k, v)
	})
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (t *batchTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, _ []byte) error {
		return cb(key)
	})
}

// Iterate returns an iterator.
func (t *batchTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	bkt, err := t.getBucket()
	if err != nil {
		return kvtx.NewErrIterator(err)
	}
	return NewIterator(bkt.Cursor(), prefix, sort, reverse)
}

// Commit commits this virtual transaction.
// If the batch is full, the underlying BoltDB tx is committed.
// Otherwise, a periodic flush goroutine ensures the batch is committed
// within flushInterval even if no more writes arrive.
func (t *batchTx) Commit(ctx context.Context) error {
	if t.done {
		return nil
	}
	t.done = true
	t.batch.pending++
	if t.batch.pending >= t.batch.batchSize {
		err := t.batch.flush()
		t.batch.mu.Unlock()
		return err
	}
	if t.batch.flushCancel == nil {
		ch := make(chan struct{})
		t.batch.flushCancel = ch
		go t.batch.timerFlush(ch)
	}
	t.batch.mu.Unlock()
	return nil
}

// Discard discards this virtual transaction.
// Rolls back the entire batch if no commits have been made.
func (t *batchTx) Discard() {
	if t.done {
		return
	}
	t.done = true
	// If this is the only pending write, roll back the BoltDB tx.
	if t.batch.pending == 0 {
		if t.batch.writeTx != nil {
			_ = t.batch.writeTx.Rollback()
			t.batch.writeTx = nil
		}
	}
	t.batch.mu.Unlock()
}

// _ is a type assertion
var _ kvtx.Store = ((*BatchStore)(nil))
var _ kvtx.Tx = ((*batchTx)(nil))
