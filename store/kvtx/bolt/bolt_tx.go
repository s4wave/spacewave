package store_kvtx_bolt

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	bdb "go.etcd.io/bbolt"
)

// Tx is a badger transaction.
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

// NewTx constructs a new badger transaction.
func NewTx(txn *bdb.Tx, bucket []byte) *Tx {
	return &Tx{txn: txn, bucket: bucket}
}

// getBucket returns the bucket
func (t *Tx) getBucket() (*bdb.Bucket, error) {
	if t.txn.Writable() {
		return t.txn.CreateBucketIfNotExists(t.bucket)
	}
	return t.txn.Bucket(t.bucket), nil
}

// Get returns values for one or more keys.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
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
	if err != nil {
		return nil, false, err
	}

	if bkt == nil {
		return nil, false, nil
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

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
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

	_ = ttl // TODO
	return bkt.Put(key, value)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	bkt, err := t.getBucket()
	if err != nil {
		return err
	}

	write := t.txn.Writable()
	var emittedKeys map[string]struct{}
	if !write {
		emittedKeys = make(map[string]struct{})
	}
	checkElem := func(k, v []byte) error {
		if len(prefix) != 0 {
			if !bytes.HasPrefix(k, prefix) {
				return nil
			}
		}

		// TODO check if we already emitted this key
		if !write {
			ks := string(k)
			if _, ok := emittedKeys[ks]; ok {
				return nil
			}
			emittedKeys[ks] = struct{}{}
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

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) error {
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
func (t *Tx) Exists(key []byte) (bool, error) {
	bkt, err := t.getBucket()
	if err != nil {
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
