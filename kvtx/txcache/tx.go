package kvtx_txcache

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/kvtx"
)

// Tx implements a read transaction backed by a TXCache.
type Tx struct {
	tc     *TXCache
	write  bool
	readTx kvtx.TxOps

	newWriteTx  func() (kvtx.Tx, error)
	closeReadTx func()
}

// NewTx constructs a new transaction.
func NewTx(store *Store, write bool) (*Tx, error) {
	readTx, err := store.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	return &Tx{
		tc:     NewTXCache(readTx, false),
		write:  write,
		readTx: readTx,

		newWriteTx: func() (kvtx.Tx, error) {
			return store.store.NewTransaction(true)
		},
		closeReadTx: func() {
			readTx.Discard()
		},
	}, nil
}

// NewTxWithCbs constructs a new transaction with read-ops and a cb to create a write transaction.
func NewTxWithCbs(
	readTx kvtx.TxOps,
	write bool,
	closeReadTx func(),
	newWriteTx func() (kvtx.Tx, error),
) (*Tx, error) {
	if newWriteTx == nil && write {
		return nil, errors.New("func to create new write tx must be set")
	}
	return &Tx{
		closeReadTx: closeReadTx,
		newWriteTx:  newWriteTx,

		readTx: readTx,
		tc:     NewTXCache(readTx, false),
		write:  write,
	}, nil
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) Commit(ctx context.Context) error {
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if t.readTx == nil || t.tc == nil {
		return kvtx.ErrDiscarded
	}
	if t.closeReadTx != nil {
		t.closeReadTx()
		t.closeReadTx = nil
	}
	t.readTx = nil

	writeTx, err := t.newWriteTx()
	if err != nil {
		return err
	}
	defer writeTx.Discard()

	ops, err := t.tc.BuildOps(false)
	t.tc = nil
	if err != nil {
		return err
	}
	for i, op := range ops {
		if err := op(writeTx); err != nil {
			return err
		}
		ops[i] = nil
	}
	return writeTx.Commit(ctx)
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	if t.closeReadTx != nil {
		t.closeReadTx()
		t.closeReadTx = nil
	}
	t.readTx = nil
	t.tc = nil
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
