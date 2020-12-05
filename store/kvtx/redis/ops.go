package store_kvtx_redis

import (
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
func (t *txOps) Get(key []byte) ([]byte, bool, error) {
	data, err := redis.Bytes(t.conn.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
			err = nil
		}
		return nil, false, err
	}

	return data, true, nil
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *txOps) Set(key, value []byte, ttl time.Duration) error {
	var err error
	wc := t.writeConn
	if ttl >= time.Second {
		_, err = wc.Do("SETEX", key, int(ttl.Seconds()), value)
	} else {
		_, err = wc.Do("SET", key, value)
	}
	return err
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *txOps) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
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
func (t *txOps) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	return t.ScanPrefixKeys(prefix, func(key []byte) error {
		keyValue, keyValueOk, err := t.Get(key)
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
func (t *txOps) Iterate(prefix []byte, sort, reverse bool) kvtx.Iterator {
	return NewIterator(t, prefix, sort, reverse)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *txOps) Delete(key []byte) error {
	_, err := t.writeConn.Do("DEL", key)
	return err
}

// Exists checks if a key exists.
func (t *txOps) Exists(key []byte) (bool, error) {
	return redis.Bool(t.conn.Do("EXISTS", key))
}

// _ is a type assertion
var _ kvtx.TxOps = ((*txOps)(nil))
