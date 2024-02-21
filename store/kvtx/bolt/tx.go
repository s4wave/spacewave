//go:build !js && !wasip1
// +build !js,!wasip1

package store_kvtx_bolt

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/aperturerobotics/hydra/kvtx"
	rbt "github.com/emirpasic/gods/trees/redblacktree"
	bdb "go.etcd.io/bbolt"
)

// Tx is a bolt transaction.
type Tx struct {
	txn           *bdb.Tx
	bucket        []byte
	discardOnce   sync.Once
	readOnlyCache []*pendingValue
}

// pendingValue is a pending write value
type pendingValue struct {
	key   []byte
	value []byte
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
		return nil, bdb.ErrBucketNotFound
	}
	return bk, nil
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	if !t.txn.Writable() {
		for _, v := range t.readOnlyCache {
			if bytes.Equal(v.key, key) {
				if len(v.value) == 0 {
					return nil, false, nil
				}
				pval := make([]byte, len(v.value))
				copy(pval, v.value)
				return pval, true, nil
			}
		}
	}

	bkt, err := t.getBucket()
	if err == bdb.ErrBucketNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	item := bkt.Get(key)
	if len(item) == 0 {
		return nil, false, nil
	}

	// item is only valid for time of transaction
	valb := make([]byte, len(item))
	copy(valb, item)
	return valb, true, nil
}

// Size returns the number of keys in the store.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	bkt, err := t.getBucket()
	if err != nil {
		return 0, err
	}
	stats := bkt.Stats()
	return uint64(stats.KeyN), nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	// we cannot store empty values in Bolt
	if len(value) == 0 {
		return kvtx.ErrEmptyValue
	}
	if !t.txn.Writable() {
		for i, v := range t.readOnlyCache {
			if bytes.Equal(v.key, key) {
				if bytes.Equal(v.value, value) {
					return nil
				}

				t.readOnlyCache[i] = t.readOnlyCache[len(t.readOnlyCache)-1]
				t.readOnlyCache[len(t.readOnlyCache)-1] = nil
				t.readOnlyCache = t.readOnlyCache[:len(t.readOnlyCache)-1]
				break
			}
		}
		pkey := make([]byte, len(key))
		copy(pkey, key)
		pval := make([]byte, len(value))
		copy(pval, value)
		t.readOnlyCache = append(t.readOnlyCache, &pendingValue{
			key:   pkey,
			value: pval,
		})
		return nil
	}

	bkt, err := t.getBucket()
	if err != nil {
		return err
	}

	return bkt.Put(key, value)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	bkt, err := t.getBucket()
	if err != nil {
		return err
	}

	write := t.txn.Writable()
	emittedKeys := rbt.NewWith(func(i, j interface{}) int {
		return bytes.Compare(i.([]byte), j.([]byte))
	})
	checkElem := func(k, v []byte) error {
		if len(prefix) != 0 {
			if !bytes.HasPrefix(k, prefix) {
				return nil
			}
		}

		if !write {
			if _, ok := emittedKeys.Get(k); ok {
				return nil
			}
			emittedKeys.Put(k, struct{}{})
		}

		return cb(k, v)
	}

	if !write {
		for _, v := range t.readOnlyCache {
			if err := checkElem(v.key, v.value); err != nil {
				return err
			}
		}
	}

	// TODO: this might be slow, we should use buckets for prefixes as an optimization
	return bkt.ForEach(checkElem)
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
func (t *Tx) Delete(ctx context.Context, key []byte) error {
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
func (t *Tx) Commit(ctx context.Context) error {
	var done bool
	var err error
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
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	bkt, err := t.getBucket()
	if err != nil {
		if err == bdb.ErrBucketNotFound {
			return false, nil
		}
		return false, err
	}

	i := bkt.Get(key)
	return len(i) != 0, nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.discardOnce.Do(func() {
		_ = t.txn.Rollback()
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
