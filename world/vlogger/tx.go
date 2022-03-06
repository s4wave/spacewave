package world_vlogger

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// Tx implements a Tx wrapped with verbose logging.
type Tx struct {
	// WorldState is the world state logger.
	*WorldState
	// Tx is the underlying Tx object.
	tx world.Tx
	// le is the logger
	le *logrus.Entry

	discardOnce sync.Once
}

// NewTx constructs a new world tx vlogger.
func NewTx(le *logrus.Entry, worldTx world.Tx) *Tx {
	return &Tx{
		WorldState: NewWorldState(le, worldTx),
		tx:         worldTx,
		le:         le,
	}
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// Will return error if called after Discard()
func (t *Tx) Commit(ctx context.Context) (err error) {
	// only log the first Commit or Discard call
	var logFn func()
	t.discardOnce.Do(func() {
		logFn = func() {
			t.le.Debugf(
				"Commit() => err(%v)",
				err,
			)
		}
	})
	if logFn != nil {
		defer logFn()
	}
	return t.tx.Commit(ctx)
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.tx.Discard()
	t.discardOnce.Do(func() {
		// t.le.Debug("Discard()")
	})
}

// _ is a type assertion
var _ world.Tx = ((*Tx)(nil))
