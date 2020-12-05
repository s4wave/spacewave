package kvtx_vlogger

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/sirupsen/logrus"
)

// Tx implements a verbose logger tx.
type Tx struct {
	iter uint32
	kvtx.Tx
	le *logrus.Entry

	discardOnce sync.Once
}

func NewTx(tx kvtx.Tx, le *logrus.Entry) *Tx {
	return &Tx{
		Tx: tx,
		le: le,
	}
}

// keyForLogging formats a key as a string suitable for logging
//
// removes non ascii chars
func keyForLogging(key []byte) string {
	return strconv.QuoteToASCII(string(key))
}

// Get returns values for a key.
func (t *Tx) Get(key []byte) (data []byte, found bool, err error) {
	defer func() {
		t.le.Debugf(
			"Get(%s) => data(%d) found(%v) err(%v)",
			keyForLogging(key),
			len(data),
			found,
			err,
		)
	}()
	return t.Tx.Get(key)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) (err error) {
	ta := time.Now()
	defer func() {
		tb := time.Now()
		dur := tb.Sub(ta).String()
		t.le.Debugf(
			"ScanPrefix(%s) => err(%v) dur(%v)",
			keyForLogging(prefix),
			err,
			dur,
		)
	}()
	return t.Tx.ScanPrefix(prefix, func(key, value []byte) error {
		ta := time.Now()
		err := cb(key, value)
		tb := time.Now()
		dur := tb.Sub(ta).String()
		t.le.Debugf(
			"ScanPrefix(%s) => callback(%v, len(%v)) => err(%v) cb-dur(%v)",
			keyForLogging(prefix),
			keyForLogging(key), len(value),
			err,
			dur,
		)
		return err
	})
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) (err error) {
	ta := time.Now()
	defer func() {
		tb := time.Now()
		dur := tb.Sub(ta).String()
		t.le.Debugf(
			"ScanPrefixKeys(%s) => err(%v) dur(%v)",
			keyForLogging(prefix),
			err,
			dur,
		)
	}()
	return t.Tx.ScanPrefixKeys(prefix, func(key []byte) error {
		ta := time.Now()
		err := cb(key)
		tb := time.Now()
		dur := tb.Sub(ta).String()
		t.le.Debugf(
			"ScanPrefixKeys(%s) => callback(%v) => err(%v) cb-dur(%v)",
			keyForLogging(prefix),
			keyForLogging(key),
			err,
			dur,
		)
		return err
	})
}

// Iterate returns an iterator with a given key prefix.
func (t *Tx) Iterate(prefix []byte, sort, reverse bool) kvtx.Iterator {
	ta := time.Now()
	ii := atomic.AddUint32(&t.iter, 1) - 1
	it := t.Tx.Iterate(prefix, sort, reverse)
	t.le.Debugf(
		"Iterate(%s) => it(%d)",
		keyForLogging(prefix),
		ii,
	)
	le := t.le.WithField("kvtx-vlogger-iter-id", ii)
	return &Iterator{ii: ii, ta: ta, it: it, le: le}
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) (err error) {
	defer func() {
		t.le.Debugf(
			"Set(%s) => value(%d) ttl(%v) err(%v)",
			keyForLogging(key),
			len(value),
			ttl,
			err,
		)
	}()
	return t.Tx.Set(key, value, ttl)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) (err error) {
	defer func() {
		t.le.Debugf(
			"Delete(%s) => err(%v)",
			keyForLogging(key),
			err,
		)
	}()
	return t.Tx.Delete(key)
}

// Exists checks if a key exists.
func (t *Tx) Exists(key []byte) (found bool, err error) {
	defer func() {
		t.le.Debugf(
			"Exists(%s) => found(%v) err(%v)",
			keyForLogging(key),
			found,
			err,
		)
	}()
	return t.Tx.Exists(key)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) (err error) {
	// only log the first Commit or Discard call
	t.discardOnce.Do(func() {
		defer func() {
			t.le.Debugf(
				"Commit() => err(%v)",
				err,
			)
		}()
	})
	return t.Tx.Commit(ctx)
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.Tx.Discard()
	t.discardOnce.Do(func() {
		t.le.Debug("Discard()")
	})
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
