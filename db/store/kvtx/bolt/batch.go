//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/db/kvtx"
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
// Write transactions buffer their operations locally and apply them to the
// shared BoltDB write tx at Commit. The real commit happens when the batch
// is full, Flush is called, or the periodic flush timer fires (100ms after
// the first unflushed write).
type BatchStore struct {
	store     *Store
	batchSize int

	mu          sync.Mutex
	writeTx     *bdb.Tx
	pending     int
	flushCancel chan struct{} // closed to cancel the periodic flush goroutine

	stats BatchStats
}

// GetDB returns the bolt DB.
func (b *BatchStore) GetDB() *bdb.DB {
	return b.store.GetDB()
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
		err := b.flush()
		b.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return b.store.NewTransaction(ctx, false)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.writeTx == nil {
		var err error
		b.writeTx, err = b.store.db.Begin(true)
		if err != nil {
			return nil, err
		}
		b.pending = 0
	}
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

// batchWrite is one buffered write operation. A nil value deletes the key.
type batchWrite struct {
	key   []byte
	value []byte
}

// batchTx is a virtual write transaction within a batch.
type batchTx struct {
	batch  *BatchStore
	bucket []byte
	done   bool
	// writes buffers this transaction's operations in order. They are
	// applied to the shared BoltDB write tx at Commit, so an abandoned
	// or discarded transaction never touches shared state and never
	// blocks other transactions or a flush.
	writes []batchWrite
}

// finalState returns this transaction's last operation per key.
func (t *batchTx) finalState() map[string]batchWrite {
	m := make(map[string]batchWrite, len(t.writes))
	for _, w := range t.writes {
		m[string(w.key)] = w
	}
	return m
}

// clone returns a copy of the byte slice.
func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	return slices.Clone(b)
}

// Set sets the value of a key.
func (t *batchTx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	t.batch.stats.Writes.Add(1)
	t.writes = append(t.writes, batchWrite{key: cloneBytes(key), value: cloneBytes(value)})
	return nil
}

// Delete deletes a key.
func (t *batchTx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	t.writes = append(t.writes, batchWrite{key: cloneBytes(key), value: nil})
	return nil
}

// Get returns values for the key, including this transaction's buffered
// writes.
func (t *batchTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	if w, ok := t.finalState()[string(key)]; ok {
		if w.value == nil {
			return nil, false, nil
		}
		return slices.Clone(w.value), true, nil
	}

	t.batch.mu.Lock()
	defer t.batch.mu.Unlock()
	if t.batch.writeTx == nil {
		return nil, false, nil
	}
	bkt := t.batch.writeTx.Bucket(t.bucket)
	if bkt == nil {
		return nil, false, nil
	}
	value := bkt.Get(key)
	if value == nil {
		return nil, false, nil
	}
	// Value is only valid during tx, clone it.
	return slices.Clone(value), true, nil
}

// Exists checks if a key exists, including this transaction's buffered
// writes.
func (t *batchTx) Exists(ctx context.Context, key []byte) (bool, error) {
	val, found, err := t.Get(ctx, key)
	if err != nil {
		return false, err
	}
	_ = val
	return found, nil
}

// Size returns the number of keys in the store, including this
// transaction's buffered writes.
func (t *batchTx) Size(ctx context.Context) (uint64, error) {
	final := t.finalState()

	t.batch.mu.Lock()
	defer t.batch.mu.Unlock()
	if t.batch.writeTx == nil {
		return uint64(len(final)), nil //nolint:gosec
	}
	bkt := t.batch.writeTx.Bucket(t.bucket)
	if bkt == nil {
		return uint64(len(final)), nil //nolint:gosec
	}
	size := uint64(bkt.Stats().KeyN) //nolint:gosec
	c := bkt.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		if w, ok := final[string(k)]; ok {
			if w.value == nil {
				size--
			}
			delete(final, string(k))
		}
	}
	for _, w := range final {
		if w.value != nil {
			size++
		}
	}
	return size, nil
}

// mergedEntries returns the merged view of the bolt bucket and this
// transaction's buffered writes for the prefix, sorted by key.
func (t *batchTx) mergedEntries(prefix []byte) ([]kvEntry, error) {
	final := t.finalState()

	t.batch.mu.Lock()
	var entries []kvEntry
	if t.batch.writeTx != nil {
		bkt := t.batch.writeTx.Bucket(t.bucket)
		if bkt != nil {
			err := bkt.ForEach(func(k, v []byte) error {
				if len(prefix) != 0 && !bytes.HasPrefix(k, prefix) {
					return nil
				}
				if w, ok := final[string(k)]; ok {
					delete(final, string(k))
					if w.value == nil {
						return nil
					}
					entries = append(entries, kvEntry{key: slices.Clone(k), value: slices.Clone(w.value)})
					return nil
				}
				entries = append(entries, kvEntry{key: slices.Clone(k), value: slices.Clone(v)})
				return nil
			})
			if err != nil {
				t.batch.mu.Unlock()
				return nil, err
			}
		}
	}
	t.batch.mu.Unlock()

	for _, w := range final {
		if len(prefix) != 0 && !bytes.HasPrefix(w.key, prefix) {
			continue
		}
		if w.value != nil {
			entries = append(entries, kvEntry(w))
		}
	}
	slices.SortFunc(entries, func(a, b kvEntry) int {
		return bytes.Compare(a.key, b.key)
	})
	return entries, nil
}

// ScanPrefix iterates over keys with a prefix.
func (t *batchTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	entries, err := t.mergedEntries(prefix)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := cb(e.key, e.value); err != nil {
			return err
		}
	}
	return nil
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (t *batchTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, _ []byte) error {
		return cb(key)
	})
}

// Iterate returns an iterator over the merged view of the bolt bucket
// and this transaction's buffered writes.
func (t *batchTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	entries, err := t.mergedEntries(prefix)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}
	if reverse {
		slices.Reverse(entries)
	}
	return newSliceIterator(entries, reverse)
}

// Commit commits this virtual transaction by applying its buffered writes
// to the shared BoltDB write tx. If the batch is full, the underlying
// BoltDB tx is committed. Otherwise, a periodic flush goroutine ensures
// the batch is committed within flushInterval even if no more writes
// arrive.
func (t *batchTx) Commit(ctx context.Context) error {
	if t.done {
		return nil
	}
	t.done = true

	b := t.batch
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.writeTx == nil {
		var err error
		b.writeTx, err = b.store.db.Begin(true)
		if err != nil {
			return err
		}
		b.pending = 0
	}
	bkt, err := b.writeTx.CreateBucketIfNotExists(t.bucket)
	if err != nil {
		return err
	}
	for _, w := range t.writes {
		if w.value == nil {
			if err := bkt.Delete(w.key); err != nil {
				return err
			}
			continue
		}
		if err := bkt.Put(w.key, w.value); err != nil {
			return err
		}
	}
	b.pending++
	if b.pending >= b.batchSize {
		return b.flush()
	}
	if b.flushCancel == nil {
		ch := make(chan struct{})
		b.flushCancel = ch
		go b.timerFlush(ch)
	}
	return nil
}

// Discard discards this virtual transaction. Its buffered writes are
// dropped; shared state is untouched.
func (t *batchTx) Discard() {
	t.done = true
	t.writes = nil
}

// _ is a type assertion
var (
	_ kvtx.Store = (*BatchStore)(nil)
	_ kvtx.Tx    = (*batchTx)(nil)
)
