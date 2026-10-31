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

// OnReinstall registers fn with the underlying engine's reinstall hooks.
func (e *Engine) OnReinstall(ctx context.Context, fn func(seqno uint64)) (func(), error) {
	return world.OnReinstall(ctx, e.Engine, fn)
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	// Open the World transaction with a logger for its transaction ID.
	txid := e.txInc.Add(1)
	le := e.le.WithField("world-vlogger-txid", txid)
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil {
		le.WithError(err).Warnf("NewTransaction(%v) errored", write)
		return nil, err
	}

	return NewTx(le, tx), nil
}

// WatchChanges forwards the change feed of the underlying engine.
func (e *Engine) WatchChanges(
	ctx context.Context,
	afterSeqno uint64,
	touches world.ChangeFilter,
	cb func(seqno uint64, changes *world.ChangeSet) error,
) error {
	return world.WatchTouchingChanges(ctx, e.Engine, afterSeqno, touches, cb)
}

// WatchIndexedChanges forwards the indexed change feed of the underlying engine.
func (e *Engine) WatchIndexedChanges(
	ctx context.Context,
	afterSeqno uint64,
	interest *world.ChangeInterest,
	cb func(seqno uint64, changes *world.ChangeSet) error,
) error {
	return world.WatchInterestedChanges(ctx, e.Engine, afterSeqno, interest, cb)
}

// _ is a type assertion
var (
	_ world.Engine               = (*Engine)(nil)
	_ world.IndexedChangeWatcher = (*Engine)(nil)
	_ world.ReinstallNotifier    = (*Engine)(nil)
)
