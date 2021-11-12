package kvtx_vlogger

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/sirupsen/logrus"
)

// VLoggerStore wraps a KVTx store to verbosely log all operations.
type VLoggerStore struct {
	txInc uint64
	kvtx.Store
	le *logrus.Entry
}

func NewVLogger(le *logrus.Entry, store kvtx.Store) *VLoggerStore {
	return &VLoggerStore{le: le, Store: store}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (l *VLoggerStore) NewTransaction(write bool) (kvtx.Tx, error) {
	txid := atomic.AddUint64(&l.txInc, 1)
	le := l.le.WithField("kvtx-vlogger-txid", txid)
	ntx, err := l.Store.NewTransaction(write)
	if err != nil {
		le.WithError(err).Warnf("NewTransaction(%v) errored", write)
		return nil, err
	}
	defer func() {
		le.Debugf("NewTransaction(%v)", write)
	}()
	return NewTx(ntx, le), nil
}

type sExec interface {
	// Execute executes the given store.
	// Returning nil ends execution.
	// Returning an error triggers a retry with backoff.
	Execute(ctx context.Context) error
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (l *VLoggerStore) Execute(ctx context.Context) error {
	if v, ok := l.Store.(sExec); ok {
		return v.Execute(ctx)
	}
	return nil
}

// _ is a type assertion
var _ kvtx.Store = ((*VLoggerStore)(nil))
