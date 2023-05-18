// Package genji implements a sql database on a kvtx store.
package kvtx_genji

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	gengine "github.com/genjidb/genji/engine"
	"github.com/genjidb/genji/engine/memoryengine"
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
	tx, err := e.store.NewTransaction(ctx, opts.Writable)
	if err != nil {
		return nil, err
	}
	return NewTx(ctx, tx, opts.Writable), nil
}

// It should ideally be optimized for writes,
// and not reside solely in memory as it will be
// used to index entire tables.
// This store is not expected to be crash safe
// or support any recovery mechanism.
// However, it might be reused multiple times.
// The implementation must ensure that the store will not impact
// the behaviour or performance of non-transient stores. This can
// be done by creating a separate database for each transient store for example.
func (e *Engine) NewTransientStore(ctx context.Context) (gengine.TransientStore, error) {
	// TODO: implement transient engine properly, for now, use a in-memory db
	eng := memoryengine.NewEngine()
	return eng.NewTransientStore(ctx)
}

// Close closes the store.
func (k *Engine) Close() error {
	return nil
}

// _ is a type assertion
var _ gengine.Engine = ((*Engine)(nil))
