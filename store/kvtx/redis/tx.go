package store_kvtx_redis

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_txcache "github.com/aperturerobotics/hydra/kvtx/txcache"
	"github.com/gomodule/redigo/redis"
)

// ErrNotWrite is returned if a read transaction is used to write.
var ErrNotWrite = kvtx.ErrNotWrite

// Tx is a redis transaction.
// NOTE: undefined behavior when a key contains a star * character
// NOTE: ScanPrefix with binary keys currently behaves incorrectly
type Tx struct {
	s          *Store
	commitOnce sync.Once
	write      bool
	ops        txOps

	// cache uses a txcache to overlay over the read conn + cache pending writes
	// this is so that ex: Set(key), Get(key) is consistent
	// note: we don't call commit on the cache
	// note: we also issue the write directly to the writeConn immediately
	// note: not used if !write
	cache *kvtx_txcache.TXCache
}

// NewTx constructs a new badger transaction.
func (s *Store) newTx(conn redis.Conn, write bool) *Tx {
	return &Tx{
		s:     s,
		write: write,
		ops:   txOps{conn: conn},
	}
}

// Get returns values for a key.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
	if t.write && t.cache != nil {
		return t.cache.Get(key)
	}

	return (&t.ops).Get(key)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
	// assert write connection exists
	_, err := t.getWriteConn()
	if err != nil {
		return err
	}

	// apply change to redis MULTI tx
	if err := (&t.ops).Set(key, value, ttl); err != nil {
		return err
	}

	// apply change to in-memory cache
	return t.cache.Set(key, value, ttl)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	if t.write && t.cache != nil {
		return t.cache.ScanPrefix(prefix, cb)
	}

	return (&t.ops).ScanPrefix(prefix, cb)
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (t *Tx) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	if t.write && t.cache != nil {
		return t.cache.ScanPrefixKeys(prefix, cb)
	}

	return (&t.ops).ScanPrefixKeys(prefix, cb)
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// If sort, iterates in sorted order, reverse reverses the key iteration.
// The prefix is NOT clipped from the output keys.
// If !sort, reverse has no effect.
// Must call Next() or Seek() before valid.
func (t *Tx) Iterate(prefix []byte, sort, reverse bool) kvtx.Iterator {
	if t.write && t.cache != nil {
		return t.cache.Iterate(prefix, sort, reverse)
	}

	return (&t.ops).Iterate(prefix, sort, reverse)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) error {
	// assert write connection exists
	_, err := t.getWriteConn()
	if err != nil {
		return err
	}
	// apply change to redis MULTI tx
	if err := (&t.ops).Delete(key); err != nil {
		return err
	}
	// apply change to in-memory cache
	return t.cache.Delete(key)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) error {
	var err error
	t.commitOnce.Do(func() {
		// execute the command
		if t.write {
			defer t.s.writeMtx.Unlock()
			wc := t.ops.writeConn
			if wc != nil {
				_, err = wc.Do("EXEC")
				_ = wc.Close()
			}
			t.cache = nil
		}
		_ = t.ops.conn.Close()
	})
	return err
}

// Exists checks if a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	return redis.Bool(t.ops.conn.Do("EXISTS", key))
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.commitOnce.Do(func() {
		if t.write {
			defer t.s.writeMtx.Unlock()
			wc := t.ops.writeConn
			if wc != nil {
				_, _ = wc.Do("DISCARD")
				_ = wc.Close()
			}
			t.cache = nil
		}
		_ = t.ops.conn.Close()
	})
}

// getWriteConn gets or establishes the write conn.
func (t *Tx) getWriteConn() (redis.Conn, error) {
	if !t.write {
		return nil, ErrNotWrite
	}
	var err error
	wc := t.ops.writeConn
	if wc != nil && wc.Err() != nil {
		_ = wc.Close()
		wc = nil
	}
	if wc == nil {
		wc, err = t.s.buildConn(t.s.ctx, true)
		t.ops.writeConn = wc
		if err != nil {
			return nil, err
		}

		// if we just re-built the conn:
		// re-play any transactions so far in the cache.
		// this recovers from a timeout mid-transaction
		if t.cache != nil {
			ops, err := t.cache.BuildOps(false)
			if err != nil {
				return nil, err
			}
			for _, op := range ops {
				if err := op(&t.ops); err != nil {
					return nil, err
				}
			}
		} else {
			t.cache = kvtx_txcache.NewTXCache(&t.ops, false)
		}
	}
	return wc, err
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
