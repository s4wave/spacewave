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
	iter atomic.Uint32
	kvtx.Tx
	le *logrus.Entry

	discardOnce sync.Once
}

func NewTx(le *logrus.Entry, tx kvtx.Tx) *Tx {
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
func (t *Tx) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	t.le.Debugf("Get(%s) start", keyForLogging(key))
	defer func() {
		t.le.Debugf(
			"Get(%s) => data(%d) found(%v) err(%v)",
			keyForLogging(key),
			len(data),
			found,
			err,
		)
	}()
	return t.Tx.Get(ctx, key)
}

// Size returns number of keys in the store.
func (t *Tx) Size(ctx context.Context) (count uint64, err error) {
	t.le.Debug("Size() start")
	defer func() {
		t.le.Debugf(
			"Size() => count(%d) err(%v)",
			count,
			err,
		)
	}()
	return t.Tx.Size(ctx)
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) (err error) {
	t.le.Debugf("ScanPrefix(%s) start", keyForLogging(prefix))
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
	return t.Tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
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
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) (err error) {
	t.le.Debugf("ScanPrefixKeys(%s) start", keyForLogging(prefix))
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
	return t.Tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
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
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	ii := t.iter.Add(1) - 1
	it := t.Tx.Iterate(ctx, prefix, sort, reverse)
	t.le.Debugf(
		"Iterate(%s, %v, %v) => it(%d)",
		keyForLogging(prefix),
		sort, reverse,
		ii,
	)
	le := t.le.WithField("kvtx-vlogger-iter-id", ii)
	return NewIterator(le, ii, it)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(ctx context.Context, key, value []byte) (err error) {
	t.le.Debugf("Set(%s) start", keyForLogging(key))
	defer func() {
		t.le.Debugf(
			"Set(%s) => value(%d) err(%v)",
			keyForLogging(key),
			len(value),
			err,
		)
	}()
	return t.Tx.Set(ctx, key, value)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(ctx context.Context, key []byte) (err error) {
	t.le.Debugf("Delete(%s) start", keyForLogging(key))
	defer func() {
		t.le.Debugf(
			"Delete(%s) => err(%v)",
			keyForLogging(key),
			err,
		)
	}()
	return t.Tx.Delete(ctx, key)
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (found bool, err error) {
	t.le.Debugf("Exists(%s) start", keyForLogging(key))
	defer func() {
		t.le.Debugf(
			"Exists(%s) => found(%v) err(%v)",
			keyForLogging(key),
			found,
			err,
		)
	}()
	return t.Tx.Exists(ctx, key)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) (err error) {
	// only log the first Commit or Discard call
	var logFn func()
	t.discardOnce.Do(func() {
		t1 := time.Now()
		logFn = func() {
			t.le.Debugf(
				"Commit() => err(%v) dur(%v)",
				err,
				time.Since(t1).String(),
			)
		}
	})
	if logFn != nil {
		defer logFn()
	}
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
