package store_kvtx_redis

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/gomodule/redigo/redis"
)

// ErrNotWrite is returned if a read transaction is used to write.
var ErrNotWrite = errors.New("not a write transaction")

// Tx is a redis transaction.
// NOTE: undefined behavior when a key contains a star * character
// not sure best way to handle this.
type Tx struct {
	s          *Store
	conn       redis.Conn
	commitOnce sync.Once
	write      bool
}

// NewTx constructs a new badger transaction.
func (s *Store) newTx(conn redis.Conn, write bool) *Tx {
	return &Tx{s: s, conn: conn, write: write}
}

// Get returns values for a key.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
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
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
	if !t.write {
		return ErrNotWrite
	}

	var err error
	if ttl >= time.Second {
		_, err = t.conn.Do("SETEX", key, int(ttl.Seconds()), value)
	} else {
		_, err = t.conn.Do("SET", key, value)
	}
	return err
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
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
func (t *Tx) Delete(key []byte) error {
	_, err := t.conn.Do("DEL", key)
	return err
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) error {
	var err error
	t.commitOnce.Do(func() {
		// execute the command
		if t.write {
			_, err = t.conn.Do("EXEC")
		}
		_ = t.conn.Close()
	})
	return err
}

// Exists checks if a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	return redis.Bool(t.conn.Do("EXISTS", key))
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.commitOnce.Do(func() {
		if t.write {
			_, _ = t.conn.Do("DISCARD")
		}
		_ = t.conn.Close()
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
