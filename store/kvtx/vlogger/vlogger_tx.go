package kvtx_vlogger

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/sirupsen/logrus"
)

// Tx implements a verbose logger tx.
type Tx struct {
	kvtx.Tx
	le *logrus.Entry
}

func NewTx(tx kvtx.Tx, le *logrus.Entry) *Tx {
	return &Tx{
		Tx: tx,
		le: le,
	}
}

// Get returns values for a key.
func (t *Tx) Get(key []byte) (data []byte, found bool, err error) {
	defer func() {
		t.le.Debugf(
			"Get(%s) => data(%d) found(%v) err(%v)",
			string(key),
			len(data),
			found,
			err,
		)
	}()
	return t.Tx.Get(key)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) (err error) {
	defer func() {
		t.le.Debugf(
			"Set(%s) => value(%d) ttl(%v) err(%v)",
			string(key),
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
			string(key),
			err,
		)
	}()
	return t.Tx.Delete(key)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) (err error) {
	defer func() {
		t.le.Debugf(
			"Commit() => err(%v)",
			err,
		)
	}()
	return t.Tx.Commit(ctx)
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.Tx.Discard()
	t.le.Debug("Discard()")
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
