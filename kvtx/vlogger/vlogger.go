package kvtx_vlogger

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/sirupsen/logrus"
)

// VLoggerStore wraps a KVTx store to verbosely log all operations.
type VLoggerStore struct {
	kvtx.Store
	le    *logrus.Entry
	txInc uint64
}

func NewVLogger(le *logrus.Entry, store kvtx.Store) *VLoggerStore {
	return &VLoggerStore{le: le, Store: store}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (l *VLoggerStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	txid := atomic.AddUint64(&l.txInc, 1)
	le := l.le.WithField("kvtx-vlogger-txid", txid)
	le.Debugf("NewTransaction(%v)", write)
	ntx, err := l.Store.NewTransaction(ctx, write)
	if err != nil {
		le.WithError(err).Warnf("NewTransaction(%v) => error", write)
		return nil, err
	}
	le.Debugf("NewTransaction(%v) => success", write)
	return NewTx(le, ntx), nil
}

// _ is a type assertion
var _ kvtx.Store = ((*VLoggerStore)(nil))
