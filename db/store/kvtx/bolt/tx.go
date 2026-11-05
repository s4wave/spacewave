//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Tx is a bolt transaction.
type Tx struct {
	txn    *bdb.Tx
	bucket []byte
	// store tracks ordered commit durability, nil outside a Store.
	store       *Store
	discardOnce atomic.Bool
}

// NewTx constructs a new bolt transaction.
func NewTx(txn *bdb.Tx, bucket []byte) *Tx {
	return &Tx{txn: txn, bucket: bucket}
}

// getBucket returns the store's bucket, creating it in a write transaction.
//
// A read transaction returns a nil bucket when no write has created it yet,
// as in a new database; readers treat a nil bucket as an empty store.
func (t *Tx) getBucket() (*bdb.Bucket, error) {
	if t.txn.Writable() {
		return t.txn.CreateBucketIfNotExists(t.bucket)
	}
	return t.txn.Bucket(t.bucket), nil
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) (out []byte, found bool, err error) {
	// Recover Bolt snapshot failures and require a nonempty lookup key.
	defer recoverBoltTxPanic(&err)
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	// Open the Bolt bucket or report that the key is absent.
	bkt, err := t.getBucket()
	if err != nil || bkt == nil {
		return nil, false, err
	}

	// Read the value from Bolt's transaction-scoped bucket view.
	value := bkt.Get(key)
	if value == nil {
		return nil, false, nil
	}

	// Clone the transaction-scoped value before returning it.
	return slices.Clone(value), true, nil
}

// Size returns the number of keys in the store.
func (t *Tx) Size(ctx context.Context) (size uint64, err error) {
	// Read the Bolt bucket statistics while translating snapshot failures.
	defer recoverBoltTxPanic(&err)
	bkt, err := t.getBucket()
	if err != nil || bkt == nil {
		return 0, err
	}
	stats := bkt.Stats()

	return uint64(stats.KeyN), nil //nolint:gosec
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(ctx context.Context, key, value []byte) (err error) {
	// Require a writable Bolt transaction and nonempty key before mutation.
	defer recoverBoltTxPanic(&err)
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.txn.Writable() {
		return kvtx.ErrNotWrite
	}

	// Acquire the Bolt bucket for the key update.
	bkt, err := t.getBucket()
	if err != nil {
		return err
	}

	return bkt.Put(key, value)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) (err error) {
	// Open the Bolt bucket for prefix scanning and translate snapshot failures.
	defer recoverBoltTxPanic(&err)
	bkt, err := t.getBucket()
	if err != nil || bkt == nil {
		return err
	}

	// Position the cursor at the requested prefix boundary.
	cur := bkt.Cursor()
	var key, value []byte
	if len(prefix) == 0 {
		key, value = cur.First()
	} else {
		key, value = cur.Seek(prefix)
	}

	// Deliver each matching key and value in sorted cursor order.
	for ; key != nil && (len(prefix) == 0 || bytes.HasPrefix(key, prefix)); key, value = cur.Next() {
		if err := cb(key, value); err != nil {
			return err
		}
	}
	return nil
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		return cb(key)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// Iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	// Open the Bolt bucket, iterating nothing when it does not exist yet.
	bkt, err := t.getBucket()
	if err != nil {
		return kvtx.NewErrIterator(err)
	}
	if bkt == nil {
		return newSliceIterator(nil, reverse)
	}

	// Walk the bucket through a prefix-bounded Bolt cursor.
	return NewIterator(bkt.Cursor(), prefix, sort, reverse)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(ctx context.Context, key []byte) (err error) {
	// Require a writable Bolt transaction and nonempty key before mutation.
	defer recoverBoltTxPanic(&err)
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.txn.Writable() {
		return kvtx.ErrNotWrite
	}

	// Acquire the Bolt bucket containing the key to delete.
	bkt, err := t.getBucket()
	if err != nil {
		return err
	}

	return bkt.Delete(key)
}

// Commit commits the transaction and returns once it is durable, or commits
// it ordered when ctx carries kvtx.WithOrderedCommitContext. It returns an
// error after Discard.
func (t *Tx) Commit(ctx context.Context) error {
	// A write a crash may lose commits ordered, as the caller marked it.
	if kvtx.IsOrderedCommitContext(ctx) {
		return t.CommitOrdered(ctx)
	}

	// Bolt serializes writers, so every ordered commit counted before this
	// one completes is flushed by its sync.
	flushes := t.store != nil && !t.store.db.NoSync
	var ordered uint64
	if flushes {
		ordered = t.store.flusher.Count()
	}
	if err := t.commit(t.txn.Commit); err != nil {
		return err
	}
	if flushes {
		t.store.flusher.MarkDurable(ordered)
	}
	return nil
}

// CommitOrdered commits the transaction with write ordering only.
func (t *Tx) CommitOrdered(ctx context.Context) error {
	if err := t.commit(t.txn.CommitOrdered); err != nil {
		return err
	}
	if t.store != nil {
		t.store.flusher.Ordered()
	}
	return nil
}

// commit finishes the transaction with commitFn unless it was discarded.
func (t *Tx) commit(commitFn func() error) (err error) {
	defer recoverBoltTxPanic(&err)
	if !t.discardOnce.CompareAndSwap(false, true) {
		return errors.New("commit called after discard")
	}
	return commitFn()
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (exists bool, err error) {
	// Require a nonempty lookup key and acquire its Bolt bucket.
	defer recoverBoltTxPanic(&err)
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	bkt, err := t.getBucket()
	if err != nil || bkt == nil {
		return false, err
	}

	// Use Bolt's nil value to distinguish an absent key.
	i := bkt.Get(key)
	return i != nil, nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	if t.discardOnce.CompareAndSwap(false, true) {
		defer func() {
			_ = recover()
		}()
		_ = t.txn.Rollback()
	}
}

func recoverBoltTxPanic(err *error) {
	if p := recover(); p != nil {
		*err = fmt.Errorf("%w: panic: %v", kvtx.ErrInvalidSnapshot, p)
	}
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*Tx)(nil)
