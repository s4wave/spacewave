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
	// Start the commit trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction")
	defer task.End()

	// Require a writable transaction.
	if e.writeTx == nil {
		e.Discard()
		return nil, tx.ErrNotWrite
	}

	// Set rel before moving the lease out of Engine state. A commit that finds
	// rel already set returns ErrDiscarded.
	if e.rel.Swap(true) {
		return nil, tx.ErrDiscarded
	}

	// Move the coordinator lease into commit-local state.
	locked := e.engine.bcast.Lock()
	if e.engine.writeTx != e {
		locked.Unlock()
		return nil, tx.ErrDiscarded
	}
	commitLease := e.lease
	e.lease = nil
	e.engine.committing++
	defer e.engine.finishCommit()
	locked.Unlock()

	// Commit block state and refresh its coordinator generation.
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

	// Validate the committed root before publication.
	if commitErr == nil {
		_, subtask = trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/validate-root")
		nroot = e.writeTx.state.GetRootRef()

		// A successful block commit must return a root.
		commitErr = nroot.Validate(false)
		subtask.End()
	}

	// Apply the committed root or detach the failed or stale writer.
	var nextRootRef *bucket.ObjectRef
	var retirement engineRetirement
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/apply-root-update")
	locked = e.engine.bcast.Lock()

	// Another Engine lifecycle path discarded this writer.
	if e.engine.writeTx != e {
		if commitErr == nil {
			commitErr = tx.ErrDiscarded
		}
	} else {
		// Apply the root only after commit and validation succeed.
		if commitErr == nil {
			nextRootRef = e.engine.head.root.GetRef().Clone()

			// Publish only when the transaction changed root data.
			if !nroot.EqualVT(nextRootRef.RootRef) {
				nextRootRef.RootRef = nroot

				// Durable-on-write modes validate that the committed root is
				// followable from durable storage, then publish it through commitFn.
				// Deferred single-writer mode postpones both steps until Sync makes
				// buffered blocks durable. Per-commit validation in that mode would
				// read the raw bucket and miss buffered, undrained blocks.
				deferred := e.engine.deferDurability && e.engine.writeCoordinator == nil
				if !deferred {
					_, commitErr = e.engine.writeBlockStore.Sync(ctx)
					if commitErr == nil {
						commitErr = e.engine.validateRootRefLocked(ctx, nextRootRef)
					}
					if errors.Is(commitErr, block.ErrNotFound) {
						commitErr = errors.Wrap(coord.ErrStaleGeneration, "validate committed root")
					}

					// Publish the durable root through the configured callback.
					if commitErr == nil && e.engine.commitFn != nil {
						commitErr = e.engine.commitFn(ctx, e.baseHeadRef, nextRootRef.Clone())
						if commitErr == nil {
							e.engine.durableHeadRef = nextRootRef.Clone()
						}
					}
				}

				// Publish the root into Engine state and capture retired resources.
				if commitErr == nil {
					retirement, commitErr = e.engine.setRootRefLocked(ctx, nextRootRef)
				}
			}
		}

		// Detach this writer even when commit or publication failed.
		if e.engine.writeTx == e {
			retirement = e.engine.beginRetirementLocked(e.detachLocked())
		}
	}

	// Release the Engine lock and drain retired transaction users.
	locked.Unlock()
	subtask.End()
	e.engine.drainRetirement(ctx, retirement)

	// Publish the coordinator event only after local head publication succeeds.
	if commitErr == nil && commitLease != nil {
		// Select the root to publish. Fall back to the current head when this
		// transaction did not produce a new root reference.
		publishRoot := nextRootRef
		if publishRoot == nil {
			locked := e.engine.bcast.Lock()
			publishRoot = e.engine.head.root.GetRef().Clone()
			locked.Unlock()
		}

		// Build the coordinator event for the written key prefix.
		event := coord.Event{
			KeyPrefixChanged: append([]byte(nil), e.engine.writeCoordKeyPrefix...),
		}

		// Attach the selected root when one is available.
		if publishRoot != nil {
			event.RootChanged = publishRoot.Clone()
		}

		// Publish the event after local head publication succeeds.
		_, commitErr = commitLease.Publish(ctx, event)
	}

	// Always release the coordinator lease after commit cleanup.
	if commitLease != nil {
		leaseErr := commitLease.Release(ctx)
		if commitErr == nil {
			commitErr = leaseErr
		}
	}

	// Return any commit or cleanup failure.
	if commitErr != nil {
		return nil, commitErr
	}

	// Return the committed root.
	return nextRootRef, nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (e *EngineTx) Discard() {
	// Stop when another lifecycle path already released this transaction.
	if e.rel.Swap(true) {
		return
	}

	// Detach under the Engine lock, then drain after unlocking.
	locked := e.engine.bcast.Lock()
	retirement := e.engine.beginRetirementLocked(e.detachLocked())
	locked.Unlock()
	e.engine.drainRetirement(context.Background(), retirement)
}

// detachLocked removes this transaction from Engine.coordinatorTxs and
// Engine.writeTx without waiting for its transaction locks or coordinator
// lease.
func (e *EngineTx) detachLocked() engineRetirement {
	// Mark the transaction discarded before removing its Engine registrations.
	e.rel.Store(true)
	delete(e.engine.coordinatorTxs, e)
	retirement := engineRetirement{
		readTx:  e.readTx,
		writeTx: e.writeTx,
		lease:   e.lease,
	}
	e.lease = nil

	// Move Engine.writeTxRel into the retirement, which releases the serialized
	// write turn after the Engine lock drops.
	if e.engine.writeTx == e {
		e.engine.writeTx = nil
		retirement.writeTxRel = e.engine.writeTxRel
		e.engine.writeTxRel = nil
	}

	// Return the detached resources for retirement outside the Engine lock.
	return retirement
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
