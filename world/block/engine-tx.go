package world_block

import (
	"context"
	"runtime/trace"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// EngineTx is an engine transaction wrapping the Tx object.
// returned by e.NewTransaction
type EngineTx struct {
	rel    atomic.Bool
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
	return e.engine.ForkBlockTransaction(ctx, true)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// If not write, returns ErrNotWrite.
func (e *EngineTx) Commit(ctx context.Context) error {
	_, err := e.CommitBlockTransaction(ctx)
	return err
}

// CommitBlockTransaction implements Commit but additionally returns the updated ObjectRef.
// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// If not write, returns ErrNotWrite.
func (e *EngineTx) CommitBlockTransaction(ctx context.Context) (*bucket.ObjectRef, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction")
	defer task.End()

	if e.writeTx == nil {
		e.Discard()
		return nil, tx.ErrNotWrite
	}

	// ensure tx is not already discarded
	// also marks the tx as discarded
	if e.rel.Swap(true) {
		// already discarded
		return nil, tx.ErrDiscarded
	}

	// commit
	var nroot *block.BlockRef
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/write-tx-commit")
	nroot, commitErr := e.writeTx.CommitBlockTransaction(taskCtx)
	subtask.End()

	// validate the new root
	if commitErr == nil {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/validate-root")
		nroot = e.writeTx.state.GetRootRef()
		// expect a non-nil ref
		commitErr = nroot.Validate(false)
		subtask.End()
	}

	var nextRootRef *bucket.ObjectRef
	// apply committed changes or rollback
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/apply-root-update")
	e.engine.rmtx.Lock()
	var relWriteTx func()
	if commitErr == nil {
		if e.engine.writeTx != e {
			// discarded mid-write
			commitErr = tx.ErrDiscarded
		} else {
			// call commitFn if set
			nextRootRef = e.engine.root.GetRef().Clone()
			// do nothing if nothing changed
			if !nroot.EqualVT(nextRootRef.RootRef) {
				nextRootRef.RootRef = nroot
				// call the commit function if set
				if e.engine.commitFn != nil {
					commitErr = e.engine.commitFn(nextRootRef.Clone())
				}
				if commitErr == nil {
					commitErr = e.engine.setRootRefLocked(ctx, nextRootRef)
				}
			}

			// clear write tx
			e.engine.writeTx = nil
			relWriteTx = e.engine.writeTxRel
			e.engine.writeTxRel = nil
		}
	}
	e.engine.rmtx.Unlock()
	subtask.End()

	if relWriteTx != nil {
		relWriteTx()
	}

	if commitErr != nil {
		return nil, commitErr
	}
	return nextRootRef, nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (e *EngineTx) Discard() {
	if !e.rel.Swap(true) {
		e.engine.rmtx.Lock()
		e.discardLocked()
		e.engine.rmtx.Unlock()
	}
}

// discardLocked is called while e.engine.rmtx.Lock is held.
func (e *EngineTx) discardLocked() {
	e.rel.Store(true)
	// e.writeTx will be nil if this is a read-only txn.
	if e.writeTx != nil {
		e.writeTx.Discard()
	}
	// check if the engine writeTx is this one.
	if e.engine.writeTx == e {
		e.engine.writeTx = nil
		e.engine.writeTxRel()
		e.engine.writeTxRel = nil
	}
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
