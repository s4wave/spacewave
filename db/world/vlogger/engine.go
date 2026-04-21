package world_vlogger

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// Engine wraps the Engine interface with a verbose logger.
type Engine struct {
	txInc atomic.Uint64
	// Engine is the underlying engine.
	world.Engine
	// le is the logger instance
	le *logrus.Entry
}

// NewEngine wraps an engine with a logger.
func NewEngine(le *logrus.Entry, eng world.Engine) *Engine {
	return &Engine{
		Engine: eng,
		le:     le,
	}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	txid := e.txInc.Add(1)
	le := e.le.WithField("world-vlogger-txid", txid)
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil {
		le.WithError(err).Warnf("NewTransaction(%v) errored", write)
		return nil, err
	}
	/*
		defer func() {
			le.Debugf("NewTransaction(%v)", write)
		}()
	*/
	return NewTx(le, tx), nil
}

// _ is a type assertion
var _ world.Engine = ((*Engine)(nil))
