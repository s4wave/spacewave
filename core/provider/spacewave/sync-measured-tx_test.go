package provider_spacewave

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
)

// syncMeasuredTx counts marker access without changing transaction semantics.
type syncMeasuredTx struct {
	// tx is the underlying transaction.
	tx kvtx.Tx
	// store receives aggregate measurements.
	store *syncMeasuredStore
	// write distinguishes acknowledgement scans from candidate acquisition.
	write bool
	// reads counts candidates examined in this read transaction.
	reads int64
	// closed records that the transaction has left the store's open count.
	closed atomic.Bool
}

// visit counts a pending marker and the current acquisition window.
func (t *syncMeasuredTx) visit(key []byte) {
	if !bytes.HasPrefix(key, []byte(pendingUploadOrderPrefix)) {
		return
	}
	t.store.visits.Add(1)
	if !t.write {
		t.reads++
		for previous := t.store.page.Load(); t.reads > previous; previous = t.store.page.Load() {
			if t.store.page.CompareAndSwap(previous, t.reads) {
				break
			}
		}
	}
}

// Size delegates the record count.
func (t *syncMeasuredTx) Size(ctx context.Context) (uint64, error) { return t.tx.Size(ctx) }

// Get counts marker point reads.
func (t *syncMeasuredTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	value, found, err := t.tx.Get(ctx, key)
	if found && err == nil {
		t.visit(key)
	}
	return value, found, err
}

// Exists delegates membership lookup.
func (t *syncMeasuredTx) Exists(ctx context.Context, key []byte) (bool, error) {
	return t.tx.Exists(ctx, key)
}

// Set delegates pending transaction mutation.
func (t *syncMeasuredTx) Set(ctx context.Context, key, value []byte) error {
	return t.tx.Set(ctx, key, value)
}

// Delete delegates pending transaction deletion.
func (t *syncMeasuredTx) Delete(ctx context.Context, key []byte) error { return t.tx.Delete(ctx, key) }

// ScanPrefix counts each visited marker before invoking the consumer.
func (t *syncMeasuredTx) ScanPrefix(ctx context.Context, prefix []byte, cb func([]byte, []byte) error) error {
	return t.tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		t.visit(key)
		return cb(key, value)
	})
}

// ScanPrefixKeys counts keys materialized by generic iterators.
func (t *syncMeasuredTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func([]byte) error) error {
	return t.tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		t.visit(key)
		return cb(key)
	})
}

// Iterate retains the backend iterator and measures consumed values.
func (t *syncMeasuredTx) Iterate(ctx context.Context, prefix []byte, sorted, reverse bool) kvtx.Iterator {
	return &syncMeasuredIterator{iterator: t.tx.Iterate(ctx, prefix, sorted, reverse), tx: t}
}

// Commit delegates the durability barrier.
func (t *syncMeasuredTx) Commit(ctx context.Context) error {
	if t.write && t.store.commitErr != nil {
		return t.store.commitErr
	}
	err := t.tx.Commit(ctx)
	t.close()
	return err
}

// Discard releases the underlying transaction.
func (t *syncMeasuredTx) Discard() {
	t.tx.Discard()
	t.close()
}

// close removes a read transaction from the store's open count once.
func (t *syncMeasuredTx) close() {
	if !t.write && t.closed.CompareAndSwap(false, true) {
		t.store.reading.Add(-1)
	}
}

// _ is a type assertion.
var _ kvtx.Tx = (*syncMeasuredTx)(nil)
