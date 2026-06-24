package world_block

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// EngineTx is an engine transaction wrapping the Tx object.
// returned by e.NewTransaction
type EngineTx struct {
	rel    atomic.Bool
	engine *Engine

	readTx      *Tx
	writeTx     *Tx
	baseHeadRef *bucket.ObjectRef
	lease       coord.WriteLease
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
	// setRootRefLocked rebuilds transactions and discards the active writeTx.
	// Keep the coordinator lease local so root publication still precedes release.
	commitLease := e.lease
	e.lease = nil

	// commit
	var nroot *block.BlockRef
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/write-tx-commit")
	nroot, commitErr := e.writeTx.CommitBlockTransaction(taskCtx)
	subtask.End()
	if commitLease != nil && isCoordinatedWriteSnapshotError(commitErr) {
		commitErr = errors.Wrap(coord.ErrStaleGeneration, "commit world blocks")
	}
	if commitErr == nil && commitLease != nil {
		if _, err := commitLease.Refresh(ctx); err != nil {
			commitErr = err
		}
	}

	// validate the new root
	if commitErr == nil {
		_, subtask = trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/validate-root")
		nroot = e.writeTx.state.GetRootRef()
		// expect a non-nil ref
		commitErr = nroot.Validate(false)
		subtask.End()
	}

	var nextRootRef *bucket.ObjectRef
	// apply committed changes or rollback
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/apply-root-update")
	e.engine.rmtx.Lock()
	var relWriteTx func()
	if e.engine.writeTx != e {
		// discarded mid-write
		if commitErr == nil {
			commitErr = tx.ErrDiscarded
		}
	} else {
		if commitErr == nil {
			// call commitFn if set
			nextRootRef = e.engine.root.GetRef().Clone()
			// do nothing if nothing changed
			if !nroot.EqualVT(nextRootRef.RootRef) {
				nextRootRef.RootRef = nroot
				// Durable-on-write modes (coordinator, and the default single-writer
				// path) publish the durable head per commit: validate the committed
				// root is followable from durable storage, then write the head via
				// commitFn. The opt-in single-writer deferred path defers both to
				// Sync, where the block barrier first makes the root's blocks
				// durable; here it only advances the in-memory root. Validating per
				// commit in the deferred path would read the raw bucket and miss the
				// buffered-but-undrained blocks (ErrNotFound).
				deferred := e.engine.deferDurability && e.engine.writeCoordinator == nil
				if !deferred {
					_, commitErr = e.engine.writeBlockStore.Sync(ctx)
					if commitErr == nil {
						commitErr = e.engine.validateRootRefLocked(ctx, nextRootRef)
					}
					if errors.Is(commitErr, block.ErrNotFound) {
						commitErr = errors.Wrap(coord.ErrStaleGeneration, "validate committed root")
					}
					if commitErr == nil && e.engine.commitFn != nil {
						commitErr = e.engine.commitFn(ctx, e.baseHeadRef, nextRootRef.Clone())
						if commitErr == nil {
							e.engine.durableHeadRef = nextRootRef.Clone()
						}
					}
				}
				if commitErr == nil {
					commitErr = e.engine.setRootRefLocked(ctx, nextRootRef)
				}
			}
		}

		// clear the owning write tx even when its commit path failed.
		e.engine.writeTx = nil
		relWriteTx = e.engine.writeTxRel
		e.engine.writeTxRel = nil
	}
	e.engine.rmtx.Unlock()
	subtask.End()

	if relWriteTx != nil {
		relWriteTx()
	}
	if commitErr == nil && commitLease != nil {
		publishRoot := nextRootRef
		if publishRoot == nil {
			e.engine.rmtx.RLock()
			publishRoot = e.engine.root.GetRef().Clone()
			e.engine.rmtx.RUnlock()
		}
		event := coord.Event{
			KeyPrefixChanged: append([]byte(nil), e.engine.writeCoordKeyPrefix...),
		}
		if publishRoot != nil {
			event.RootChanged = publishRoot.Clone()
		}
		_, commitErr = commitLease.Publish(ctx, event)
	}
	if commitLease != nil {
		leaseErr := commitLease.Release(ctx)
		if commitErr == nil {
			commitErr = leaseErr
		}
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
	if e.readTx != nil {
		e.readTx.Discard()
		e.readTx = nil
	}
	if e.lease != nil {
		_ = e.lease.Release(context.Background())
		e.lease = nil
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

// Sync fences durable storage and advances the durable head via the engine.
func (e *EngineTx) Sync(ctx context.Context) (bool, error) {
	return e.engine.Sync(ctx)
}

// _ is a type assertion
var (
	_ world.Tx                 = ((*EngineTx)(nil))
	_ world.WorldState         = ((*EngineTx)(nil))
	_ world.ForkableWorldState = ((*EngineTx)(nil))
)
