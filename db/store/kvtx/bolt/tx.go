//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	bdb "github.com/aperturerobotics/bbolt"
	bdberrors "github.com/aperturerobotics/bbolt/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Tx is a bolt transaction.
type Tx struct {
	txn         *bdb.Tx
	bucket      []byte
	discardOnce sync.Once
}

// NewTx constructs a new bolt transaction.
func NewTx(txn *bdb.Tx, bucket []byte) *Tx {
	return &Tx{txn: txn, bucket: bucket}
}

// getBucket returns the bucket
func (t *Tx) getBucket() (*bdb.Bucket, error) {
	if t.txn.Writable() {
		return t.txn.CreateBucketIfNotExists(t.bucket)
	}
	bk := t.txn.Bucket(t.bucket)
	if bk == nil {
		return nil, bdberrors.ErrBucketNotFound
	}
	return bk, nil
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) (out []byte, found bool, err error) {
	defer recoverBoltTxPanic(&err)
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	bkt, err := t.getBucket()
	if err == bdberrors.ErrBucketNotFound {
		return nil, false, nil
	}
	if err != nil {
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
	defer recoverBoltTxPanic(&err)
	bkt, err := t.getBucket()
	if err != nil {
		return 0, err
	}
	stats := bkt.Stats()

	return uint64(stats.KeyN), nil //nolint:gosec
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(ctx context.Context, key, value []byte) (err error) {
	defer recoverBoltTxPanic(&err)
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.txn.Writable() {
		return kvtx.ErrNotWrite
	}

	bkt, err := t.getBucket()
	if err != nil {
		return err
	}

	return bkt.Put(key, value)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) (err error) {
	defer recoverBoltTxPanic(&err)
	bkt, err := t.getBucket()
	if err != nil {
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
	bkt, err := t.getBucket()
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	return NewIterator(bkt.Cursor(), prefix, sort, reverse)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(ctx context.Context, key []byte) (err error) {
	defer recoverBoltTxPanic(&err)
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	bkt, err := t.getBucket()
	if err != nil {
		return err
	}

	return bkt.Delete(key)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) (err error) {
	defer recoverBoltTxPanic(&err)
	var done bool
	t.discardOnce.Do(func() {
		err = t.txn.Commit()
		done = true
	})
	if err != nil {
		return err
	}
	if !done {
		return errors.New("commit called after discard")
	}
	return nil
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (exists bool, err error) {
	defer recoverBoltTxPanic(&err)
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

	// Use Bolt's nil value to distinguish an absent key.
	i := bkt.Get(key)
	return i != nil, nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.discardOnce.Do(func() {
		defer func() {
			_ = recover()
		}()
		_ = t.txn.Rollback()
	})
}

func recoverBoltTxPanic(err *error) {
	if p := recover(); p != nil {
		*err = fmt.Errorf("%w: panic: %v", kvtx.ErrInvalidSnapshot, p)
	}
}

// _ is a type assertion
var _ kvtx.Tx = (*Tx)(nil)
