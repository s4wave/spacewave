package store_kvtx_redis

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/gomodule/redigo/redis"
)

// txOps implements the transaction operations against a redis conn.
type txOps struct {
	conn redis.Conn
	// we can't open a connection with MULTI for writes without
	// deferring all GETS as well. to solve this, create a second conn
	// for the Writes.
	writeConn redis.Conn
}

// Get returns values for a key.
func (t *txOps) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	data, err := redis.Bytes(t.conn.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
			err = nil
		}
		return nil, false, err
	}

	return data, true, nil
}

// Size returns the number of keys in the store.
func (t *txOps) Size(ctx context.Context) (uint64, error) {
	return redis.Uint64(t.conn.Do("DBSIZE"))
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *txOps) Set(ctx context.Context, key, value []byte) error {
	wc := t.writeConn
	_, err := wc.Do("SET", key, value)
	return err
}

// SetWithTTL sets the value of a key with a ttl.
// This will not be committed until Commit is called.
func (t *txOps) SetWithTTL(ctx context.Context, key, value []byte, ttl time.Duration) error {
	wc := t.writeConn
	_, err := wc.Do("SETEX", key, int(ttl.Seconds()), value)
	return err
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *txOps) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	var iter int
	scanPrefix := append(escapeKey(prefix, 1), '*')
	for {
		vals, err := redis.Values(t.conn.Do("SCAN", iter, "MATCH", scanPrefix))
		if err != nil {
			return err
		}

		iter, _ = redis.Int(vals[0], nil)
		k, _ := redis.ByteSlices(vals[1], nil)

		for _, key := range k {
			if err := cb(key); err != nil {
				return err
			}
		}

		if iter == 0 {
			break
		}
	}

	return nil
}

// ScanPrefix iterates over keys with a prefix.
func (t *txOps) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		keyValue, keyValueOk, err := t.Get(ctx, key)
		if err != nil {
			return err
		}
		if !keyValueOk {
			return nil
		}
		return cb(key, keyValue)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse has no effect.
// Must call Next() or Seek() before valid.
func (t *txOps) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return NewIterator(ctx, t, prefix, sort, reverse)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *txOps) Delete(ctx context.Context, key []byte) error {
	_, err := t.writeConn.Do("DEL", key)
	return err
}

// Exists checks if a key exists.
func (t *txOps) Exists(ctx context.Context, key []byte) (bool, error) {
	return redis.Bool(t.conn.Do("EXISTS", key))
}

// _ is a type assertion
var _ kvtx.TxOps = ((*txOps)(nil))
