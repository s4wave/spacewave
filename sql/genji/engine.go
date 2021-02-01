// Package genji implements a sql database on a kvtx store.
package kvtx_genji

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	gengine "github.com/genjidb/genji/engine"
)

// Engine implements the GenjiDB Engine interface with a kvtx Store.
type Engine struct {
	// store is the kvtx store
	store kvtx.Store
}

// NewEngine constructs a new genji Engine wrapper.
func NewEngine(store kvtx.Store) *Engine {
	return &Engine{store: store}
}

// Begin returns a read-only or read/write transaction.
func (e *Engine) Begin(ctx context.Context, opts gengine.TxOptions) (gengine.Transaction, error) {
	tx, err := e.store.NewTransaction(opts.Writable)
	if err != nil {
		return nil, err
	}
	return NewTx(ctx, tx, opts.Writable), nil
}

// Close closes the store.
func (k *Engine) Close() error {
	return nil
}

// _ is a type assertion
var _ gengine.Engine = ((*Engine)(nil))
