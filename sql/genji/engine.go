// Package genji implements a sql database on a kvtx store.
package kvtx_genji

import (
	"context"
	"errors"

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
	tx, err := e.store.NewTransaction(opts.Writable)
	if err != nil {
		return nil, err
	}
	return NewTx(ctx, tx, opts.Writable), nil
}

// A transient engine is a database used to create temporary indices. It should
// ideally be optimized for writes, and not reside solely in memory as it will
// be used to index entire tables. This database is not expected to be crash
// safe or support any recovery mechanism, as the Commit method will never be
// used. However, it might be reused across multiple transactions.
func (e *Engine) NewTransientEngine(ctx context.Context) (gengine.Engine, error) {
	// TODO: implement transient engine properly, for now, use a in-memory db
	eng := memoryengine.NewEngine()
	return eng.NewTransientEngine(ctx)
}

// Drop releases any resource (files, memory, etc.) used by a transient database.
// It must return an error if the engine has not been created
// with NewTransientEngine.
func (e *Engine) Drop(ctx context.Context) error {
	return errors.New("not a transient engine")
}

// Close closes the store.
func (k *Engine) Close() error {
	return nil
}

// _ is a type assertion
var _ gengine.Engine = ((*Engine)(nil))
