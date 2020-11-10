package kvtx_txcache

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
)

// Tx implements a read transaction backed by a TXCache.
type Tx struct {
	store  *Store
	tc     *TXCache
	readTx kvtx.Tx
	write  bool
}

// NewTx constructs a new transaction.
func NewTx(store *Store, write bool) (*Tx, error) {
	readTx, err := store.store.NewTransaction(false)
	if err != nil {
		return nil, err
	}
	return &Tx{
		store:  store,
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
	t.readTx.Discard()
	t.readTx = nil

	writeTx, err := t.store.store.NewTransaction(true)
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
	if t.readTx != nil {
		t.readTx.Discard()
	}
	t.tc = nil
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
