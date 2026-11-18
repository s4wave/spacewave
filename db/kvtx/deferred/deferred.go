// Package kvtx_deferred holds writes to a transaction in memory and applies
// them as one write batch.
package kvtx_deferred

import (
	"bytes"
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// Default bounds on the held writes before Ops applies them.
const (
	// DefaultMaxEntries is the default number of held keys.
	DefaultMaxEntries = 4096
	// DefaultMaxBytes is the default size of the held keys and values.
	DefaultMaxBytes = 4 << 20
)

// Ops holds the writes made through it and applies them to the underlying
// transaction in one write batch on Flush, or when the held writes pass a
// bound. Reads see the held writes, and sorted iteration merges them with the
// underlying keys, so a reader between writes does not force a flush.
//
// Use it where each write batch has a fixed cost, such as a content-hashed tree
// that rehashes the path to its root on every batch. Callers must Flush before
// they read the underlying transaction directly or write it out.
//
// When the batch is a kvtx.WriteBatchPrefetcher, a held write reads the pages
// its batch will read under the writer's context, so a Flush that must not
// wait on storage, such as one under a writer lock, finds them in memory.
//
// A failed flush leaves the underlying transaction partly written. Ops returns
// the error from every later call; discard the underlying transaction.
type Ops struct {
	// underlying is the transaction the writes apply to.
	underlying kvtx.TxOps
	// batch applies write batches to underlying.
	batch kvtx.WriteBatchTxOps
	// prefetch reads the pages a batch of held keys will read, or is nil.
	prefetch kvtx.WriteBatchPrefetcher
	// maxEntries and maxBytes bound the held writes.
	maxEntries, maxBytes int

	// mtx guards the fields below.
	mtx sync.RWMutex
	// held is the held writes ordered by key.
	held *btree.BTreeG[*entry]
	// heldBytes is the size of the held keys and values.
	heldBytes int
	// err is the sticky flush error.
	err error
}

// entry is a held write.
type entry struct {
	key, value []byte
	del        bool
}

// entryLess orders entries by key.
func entryLess(a, b *entry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// NewOps constructs Ops over a transaction that supports write batches.
//
// Non-positive bounds use the defaults.
func NewOps(underlying kvtx.TxOps, batch kvtx.WriteBatchTxOps, maxEntries, maxBytes int) *Ops {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	prefetch, _ := batch.(kvtx.WriteBatchPrefetcher)
	return &Ops{
		underlying: underlying,
		batch:      batch,
		prefetch:   prefetch,
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		held:       btree.NewBTreeGOptions(entryLess, btree.Options{NoLocks: true}),
	}
}

// Held returns the number of held writes.
func (o *Ops) Held() int {
	o.mtx.RLock()
	defer o.mtx.RUnlock()
	return o.held.Len()
}

// Flush applies the held writes to the underlying transaction.
func (o *Ops) Flush(ctx context.Context) error {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	return o.flushLocked(ctx)
}

// flushLocked applies the held writes while mtx is held.
func (o *Ops) flushLocked(ctx context.Context) error {
	// Stop at an earlier failure, and skip the batch when nothing is held.
	if o.err != nil {
		return o.err
	}
	if o.held.Len() == 0 {
		return nil
	}

	// Apply the held writes in key order as one batch.
	entries := make([]kvtx.WriteBatchEntry, 0, o.held.Len())
	o.held.Scan(func(e *entry) bool {
		entries = append(entries, kvtx.WriteBatchEntry{Key: e.key, Value: e.value, Delete: e.del})
		return true
	})
	if err := o.batch.ApplyWriteBatch(ctx, entries); err != nil {
		o.err = err
		return err
	}
	o.held.Clear()
	o.heldBytes = 0
	return nil
}

// ApplyWriteBatch holds the entries, applying them all once a bound is passed.
func (o *Ops) ApplyWriteBatch(ctx context.Context, entries []kvtx.WriteBatchEntry) error {
	// Reject an empty key before holding any entry of the batch.
	for _, e := range entries {
		if len(e.Key) == 0 {
			return kvtx.ErrEmptyKey
		}
	}

	// Read the pages the batch applying the entries will read, leaving a
	// failed read to the batch.
	if o.prefetch != nil {
		keys := make([][]byte, len(entries))
		for i, e := range entries {
			keys[i] = e.Key
		}
		_ = o.prefetch.PrefetchWriteBatch(ctx, keys)
	}

	// Hold copies of the entries, the last entry for a key winning.
	o.mtx.Lock()
	defer o.mtx.Unlock()
	if o.err != nil {
		return o.err
	}
	for _, e := range entries {
		o.holdLocked(&entry{key: bytes.Clone(e.Key), value: bytes.Clone(e.Value), del: e.Delete})
	}

	// Apply the held writes once they pass a bound.
	if o.held.Len() >= o.maxEntries || o.heldBytes >= o.maxBytes {
		return o.flushLocked(ctx)
	}
	return nil
}

// holdLocked holds e, replacing an earlier write to its key.
func (o *Ops) holdLocked(e *entry) {
	if prev, ok := o.held.Set(e); ok {
		o.heldBytes -= len(prev.key) + len(prev.value)
	}
	o.heldBytes += len(e.key) + len(e.value)
}

// Set holds a write of value to key.
func (o *Ops) Set(ctx context.Context, key, value []byte) error {
	if value == nil {
		value = []byte{}
	}
	return o.ApplyWriteBatch(ctx, []kvtx.WriteBatchEntry{{Key: key, Value: value}})
}

// Delete holds a delete of key.
func (o *Ops) Delete(ctx context.Context, key []byte) error {
	return o.ApplyWriteBatch(ctx, []kvtx.WriteBatchEntry{{Key: key, Delete: true}})
}

// lookup returns the held write to key, if any.
func (o *Ops) lookup(key []byte) (*entry, bool, error) {
	// Read the held tree under the lock, failing after a failed flush.
	o.mtx.RLock()
	defer o.mtx.RUnlock()
	if o.err != nil {
		return nil, false, o.err
	}
	e, ok := o.held.Get(&entry{key: key})
	return e, ok, nil
}

// Get returns the value of key.
func (o *Ops) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	e, ok, err := o.lookup(key)
	if err != nil || ok {
		if err != nil || e.del {
			return nil, false, err
		}
		return e.value, true, nil
	}
	return o.underlying.Get(ctx, key)
}

// GetBatch returns the values of keys, reading the keys without held writes
// from the underlying transaction in one batch.
func (o *Ops) GetBatch(ctx context.Context, keys [][]byte) ([][]byte, []bool, error) {
	// Size the results and the list of keys left for the underlying read.
	values := make([][]byte, len(keys))
	found := make([]bool, len(keys))
	var missKeys [][]byte
	var missIdx []int

	// Resolve the keys with held writes.
	o.mtx.RLock()
	err := o.err
	for i, key := range keys {
		if len(key) == 0 {
			err = kvtx.ErrEmptyKey
		}
		if err != nil {
			break
		}
		if e, ok := o.held.Get(&entry{key: key}); ok {
			if !e.del {
				values[i], found[i] = e.value, true
			}
			continue
		}
		missKeys = append(missKeys, key)
		missIdx = append(missIdx, i)
	}
	o.mtx.RUnlock()
	if err != nil {
		return nil, nil, err
	}

	// Read the rest from the underlying transaction.
	if len(missKeys) == 0 {
		return values, found, nil
	}
	mv, mf, err := kvtx.GetBatch(ctx, o.underlying, missKeys)
	if err != nil {
		return nil, nil, err
	}
	for j, i := range missIdx {
		values[i], found[i] = mv[j], mf[j]
	}
	return values, found, nil
}

// Exists checks if key exists.
func (o *Ops) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	e, ok, err := o.lookup(key)
	if err != nil || ok {
		return ok && !e.del, err
	}
	return o.underlying.Exists(ctx, key)
}

// Size flushes the held writes and returns the number of keys.
func (o *Ops) Size(ctx context.Context) (uint64, error) {
	if err := o.Flush(ctx); err != nil {
		return 0, err
	}
	return o.underlying.Size(ctx)
}

// ScanPrefix iterates over the keys with prefix in sorted order.
func (o *Ops) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	// Scan the underlying transaction directly when nothing is held.
	if o.Held() == 0 {
		return o.underlying.ScanPrefix(ctx, prefix, cb)
	}

	// Deliver the merged entries in key order.
	it := o.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		value, err := it.Value()
		if err != nil {
			return err
		}
		if err := cb(it.Key(), value); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanPrefixKeys iterates over the keys with prefix in sorted order.
func (o *Ops) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	// Scan the underlying transaction directly when nothing is held.
	if o.Held() == 0 {
		return o.underlying.ScanPrefixKeys(ctx, prefix, cb)
	}

	// Deliver the merged keys in key order.
	it := o.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		if err := cb(it.Key()); err != nil {
			return err
		}
	}
	return it.Err()
}

// Iterate returns an iterator over the keys with prefix.
//
// A sorted forward iterator merges the held writes into the underlying keys.
// Other iterators flush the held writes first.
func (o *Ops) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	// Snapshot the held writes for a sorted forward iterator.
	o.mtx.Lock()
	if o.err != nil {
		err := o.err
		o.mtx.Unlock()
		return kvtx.NewErrIterator(err)
	}
	if o.held.Len() == 0 || !sort || reverse {
		err := o.flushLocked(ctx)
		o.mtx.Unlock()
		if err != nil {
			return kvtx.NewErrIterator(err)
		}
		return o.underlying.Iterate(ctx, prefix, sort, reverse)
	}
	held := o.held.Copy()
	o.mtx.Unlock()
	return newMergeIterator(o.underlying.Iterate(ctx, prefix, true, false), held, prefix)
}

// _ is a type assertion
var (
	_ kvtx.TxOps           = (*Ops)(nil)
	_ kvtx.BatchTxOps      = (*Ops)(nil)
	_ kvtx.WriteBatchTxOps = (*Ops)(nil)
)
