package world_block

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// EngineTx is an engine transaction wrapping the Tx object.
// returned by e.NewTransaction
type EngineTx struct {
	rel    uint32
	engine *Engine

	writeTx *Tx
}

// newEngineTx constructs a new EngineTx.
func newEngineTx(e *Engine, writeTx *Tx) *EngineTx {
	return &EngineTx{writeTx: writeTx, engine: e}
}

// Fork forks the current world state into a completely separate world state.
//
// Creates a new block transaction.
func (e *EngineTx) Fork(ctx context.Context) (world.WorldState, error) {
	return e.engine.ForkBlockTransaction(true)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// If not write, returns ErrNotWrite.
func (e *EngineTx) Commit(ctx context.Context) error {
	if e.writeTx == nil {
		return tx.ErrNotWrite
	}

	// ensure tx is not already discarded
	// also marks the tx as discarded
	if !e.release() {
		return tx.ErrDiscarded
	}

	// commit
	commitErr := e.writeTx.Commit(e.engine.ctx)

	// validate the new root
	var nroot *block.BlockRef
	if commitErr == nil {
		nroot = e.writeTx.state.GetRootRef()
		commitErr = nroot.Validate()
	}

	// apply committed changes or rollback
	e.engine.rmtx.Lock()
	if e.engine.writeTx != e {
		// discarded mid-write
		if commitErr == nil {
			commitErr = tx.ErrDiscarded
		}
	} else {
		e.engine.writeTx = nil // clear write tx
		// call commitFn if set
		if commitErr == nil {
			nextRootRef := e.engine.root.GetRef().Clone()
			nextRootRef.RootRef = nroot
			// call the commit function if set
			if e.engine.commitFn != nil {
				commitErr = e.engine.commitFn(nextRootRef.Clone())
			}
			if commitErr == nil {
				commitErr = e.engine.setRootRefLocked(ctx, nextRootRef)
			}
		}
	}
	e.engine.rmtx.Unlock()
	e.engine.wmtx.Release(1)

	return commitErr
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (e *EngineTx) Discard() {
	if e.release() {
		if e.writeTx != nil {
			e.writeTx.Discard()
			e.engine.wmtx.Release(1)
		}
	}
}

// release releases the tx
func (e *EngineTx) release() bool {
	rel := atomic.SwapUint32(&e.rel, 1)
	return rel != 1
}

// GetReadOnly returns if the state is read-only.
func (e *EngineTx) GetReadOnly() bool {
	return e.writeTx == nil
}

// _ is a type assertion
var (
	_ world.Tx                 = ((*EngineTx)(nil))
	_ world.WorldState         = ((*EngineTx)(nil))
	_ world.ForkableWorldState = ((*EngineTx)(nil))
)
