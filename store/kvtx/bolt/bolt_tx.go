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
	txn         *bdb.Tx
	bucket      []byte
	discardOnce sync.Once
}

// NewTx constructs a new badger transaction.
func NewTx(txn *bdb.Tx, bucket []byte) *Tx {
	return &Tx{txn: txn, bucket: bucket}
}

// getBucket returns the bucket
func (t *Tx) getBucket() (*bdb.Bucket, error) {
	return t.txn.CreateBucketIfNotExists(t.bucket)
}

// Get returns values for one or more keys.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
	bkt, err := t.getBucket()
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

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
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

	// TODO: this might be slow, we should use buckets for prefixes as an optimization
	return bkt.ForEach(func(k []byte, v []byte) error {
		if len(prefix) != 0 {
			if !bytes.HasPrefix(k, prefix) {
				return nil
			}
		}
		return cb(k, v)
	})
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
