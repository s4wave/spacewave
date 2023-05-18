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

	commitWriteTx bool
	newWriteTx    func() (kvtx.Tx, error)
	closeReadTx   func()
}

// NewTx constructs a new transaction.
func NewTx(ctx context.Context, store *Store, write bool) (*Tx, error) {
	readTx, err := store.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	return &Tx{
		tc:     NewTXCache(readTx, false),
		write:  write,
		readTx: readTx,

		commitWriteTx: true,
		newWriteTx: func() (kvtx.Tx, error) {
			return store.store.NewTransaction(ctx, true)
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
	commitWriteTx bool,
) (*Tx, error) {
	if newWriteTx == nil && write {
		return nil, errors.New("func to create new write tx must be set")
	}
	return &Tx{
		commitWriteTx: commitWriteTx,
		closeReadTx:   closeReadTx,
		newWriteTx:    newWriteTx,

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
	if t.commitWriteTx {
		defer writeTx.Discard()
	}

	ops, err := t.tc.BuildOps(ctx, false)
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
	if !t.commitWriteTx {
		return nil
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
