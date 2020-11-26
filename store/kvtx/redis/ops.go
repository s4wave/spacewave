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

// ScanPrefix iterates over keys with a prefix.
func (t *txOps) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	var iter int
	scanPrefix := append(prefix, '*')
	for {
		vals, err := redis.Values(t.conn.Do("SCAN", iter, "MATCH", scanPrefix))
		if err != nil {
			return err
		}

		iter, _ = redis.Int(vals[0], nil)
		k, _ := redis.ByteSlices(vals[1], nil)

		for _, key := range k {
			keyValue, keyValueOk, err := t.Get(key)
			if err != nil {
				return err
			}
			if keyValueOk {
				if err := cb(key, keyValue); err != nil {
					return err
				}
			}
		}

		if iter == 0 {
			break
		}
	}

	return nil
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
