package kvtx_hidalgo

import (
	"bytes"
	"context"
	"slices"

	kv "github.com/aperturerobotics/cayley/kv/flat"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Buffer only transactions advertising page-coalesced writes. In particular,
// native Bolt and remote scalar transactions keep their existing direct path.
// These bounds cover retained keys, values and a conservative per-entry charge;
// they are not a whole-process memory limit or a delayed durability policy.
const (
	writeBatchMaxEntries = 128
	writeBatchMaxBytes   = 256 << 10
	writeBatchEntryCost  = 96
)

// bufferedTx buffers Cayley index mutations in a bounded pending overlay and
// applies them as write batches against a capable underlying transaction. It
// exists only when the wrapped transaction supports write batches.
type bufferedTx struct {
	*Tx
	// batch is the underlying transaction's write-batch capability.
	batch kvtx.WriteBatchTxOps
	// pending holds the not-yet-flushed entries keyed by string(key).
	pending map[string]kvtx.WriteBatchEntry
	// bytes is the charged size of the pending entries.
	bytes int
	// err is the sticky failure from the last flush or oversized apply.
	err error
	// closed indicates the transaction is committed or closed.
	closed bool
}

// check returns the transaction's sticky error, closed state, or ctx error.
func (t *bufferedTx) check(ctx context.Context) error {
	if t.closed {
		return kvtx.ErrDiscarded
	}
	if t.err != nil {
		return t.err
	}
	return ctx.Err()
}

// Get consults the pending overlay first, then the underlying transaction.
func (t *bufferedTx) Get(ctx context.Context, key kv.Key) (kv.Value, error) {
	if err := t.check(ctx); err != nil {
		return nil, err
	}
	if entry, ok := t.pending[string(key)]; ok {
		if entry.Delete {
			return nil, kv.ErrNotFound
		}
		return kv.Value(bytes.Clone(entry.Value)), nil
	}
	return t.Tx.Get(ctx, key)
}

// GetBatch resolves overlay hits locally and delegates only the missing keys
// to the underlying transaction's batch read.
func (t *bufferedTx) GetBatch(ctx context.Context, keys []kv.Key) ([]kv.Value, error) {
	if err := t.check(ctx); err != nil {
		return nil, err
	}
	values := make([]kv.Value, len(keys))
	missing := make([]kv.Key, 0, len(keys))
	indexes := make([]int, 0, len(keys))
	for i, key := range keys {
		if entry, ok := t.pending[string(key)]; ok {
			if !entry.Delete {
				values[i] = kv.Value(bytes.Clone(entry.Value))
			}
		} else {
			missing = append(missing, key)
			indexes = append(indexes, i)
		}
	}
	if len(missing) != 0 {
		lower, err := t.Tx.GetBatch(ctx, missing)
		if err != nil {
			return nil, err
		}
		for i, index := range indexes {
			values[index] = lower[i]
		}
	}
	return values, nil
}

// Put queues a value replacement in the pending overlay.
func (t *bufferedTx) Put(ctx context.Context, key kv.Key, value kv.Value) error {
	return t.queue(ctx, kvtx.WriteBatchEntry{Key: key, Value: value})
}

// Del queues a deletion in the pending overlay.
func (t *bufferedTx) Del(ctx context.Context, key kv.Key) error {
	return t.queue(ctx, kvtx.WriteBatchEntry{Key: key, Delete: true})
}

// batchEntryBytes computes the retention charge for one entry, counting the
// key twice: once in the entry and once in the map's string index.
func batchEntryBytes(entry kvtx.WriteBatchEntry) int {
	// Key is retained in both the entry and the map's string index.
	return 2*len(entry.Key) + len(entry.Value) + writeBatchEntryCost
}

// queue validates the key, flushes when the next entry would exceed the
// retention bounds, and records the entry in the pending overlay.
func (t *bufferedTx) queue(ctx context.Context, entry kvtx.WriteBatchEntry) error {
	if len(entry.Key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if err := t.check(ctx); err != nil {
		return err
	}

	charge := batchEntryBytes(entry)
	old, replacing := t.pending[string(entry.Key)]
	nextBytes := t.bytes + charge
	if replacing {
		nextBytes -= batchEntryBytes(old)
	}
	// Flush before recording the entry when it would exceed either bound.
	if nextBytes > writeBatchMaxBytes || (!replacing && len(t.pending) >= writeBatchMaxEntries) {
		if err := t.flush(ctx); err != nil {
			return err
		}
	}

	// An oversized value is borrowed only for this synchronous operation;
	// it never expands the bounded retained batch.
	if charge > writeBatchMaxBytes {
		t.err = t.batch.ApplyWriteBatch(ctx, []kvtx.WriteBatchEntry{entry})
		return t.err
	}

	if t.pending == nil {
		t.pending = make(map[string]kvtx.WriteBatchEntry)
	}
	key := string(entry.Key)
	if previous, ok := t.pending[key]; ok {
		t.bytes -= batchEntryBytes(previous)
	}
	entry.Key = bytes.Clone(entry.Key)
	// Keep a present empty value non-nil in GetBatch, whose nil sentinel is
	// reserved for absent keys. Scalar Get still uses its explicit error.
	entry.Value = append([]byte{}, entry.Value...)
	t.pending[key] = entry
	t.bytes += charge
	return nil
}

// flush applies the pending entries as one sorted write batch and clears the
// overlay. A failed flush is sticky: never retry it or commit a tail after it.
func (t *bufferedTx) flush(ctx context.Context) error {
	if err := t.check(ctx); err != nil {
		return err
	}
	if len(t.pending) == 0 {
		return nil
	}

	entries := make([]kvtx.WriteBatchEntry, 0, len(t.pending))
	for _, entry := range t.pending {
		entries = append(entries, entry)
	}
	// Stable ordering makes diagnostics and alternative implementations
	// deterministic; the tree's API independently handles arbitrary order.
	slices.SortFunc(entries, func(a, b kvtx.WriteBatchEntry) int { return bytes.Compare(a.Key, b.Key) })
	t.err = t.batch.ApplyWriteBatch(ctx, entries)
	t.pending = nil
	t.bytes = 0
	// A failed flush may have partially changed its enclosing transaction.
	// Keep the error sticky: never retry it or commit a successful-looking tail.
	return t.err
}

// Scan flushes pending writes, then lends the underlying transaction's
// iterator so it sees all preceding writes.

func (t *bufferedTx) Scan(ctx context.Context, opts ...kv.IteratorOption) kv.Iterator {
	// Flush before lending the iterator so it sees all preceding writes,
	// without adding a second merged-iterator/snapshot implementation.
	if err := t.flush(ctx); err != nil {
		return &batchErrorIterator{err: err}
	}
	// Scan is lazy and Reset starts a new snapshot. A caller may mutate after
	// Scan but before Next, or before Reset; fence those writes as well.
	return t.scan(ctx, t.flush, opts...)
}

// Commit flushes pending writes, then commits the underlying transaction.
func (t *bufferedTx) Commit(ctx context.Context) error {
	if err := t.flush(ctx); err != nil {
		return err
	}
	t.closed = true
	return t.Tx.Commit(ctx)
}

// Close drops pending input and discards the delegated transaction. Earlier
// capacity flushes may already have mutated the enclosing transaction.
func (t *bufferedTx) Close() error {
	t.closed = true
	t.pending = nil
	t.bytes = 0
	return t.Tx.Close()
}

// batchErrorIterator is a kv.Iterator that yields nothing and reports
// the flush error that prevented the scan from starting.
type batchErrorIterator struct{ err error }

func (i *batchErrorIterator) Next(context.Context) bool { return false }
func (i *batchErrorIterator) Err() error                { return i.err }
func (i *batchErrorIterator) Key() kv.Key               { return nil }
func (i *batchErrorIterator) Val() kv.Value             { return nil }
func (i *batchErrorIterator) Reset()                    {}
func (i *batchErrorIterator) Close() error              { return nil }

var _ kv.Tx = (*bufferedTx)(nil)
