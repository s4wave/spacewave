package sobject_world_engine

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// executeWatchSOState watches and processes shared object state changes.
func (c *Controller) executeWatchSOState(
	ctx context.Context,
	soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	soEngine *soEngine,
) error {
	var snap sobject.SharedObjectStateSnapshot
	var err error
	for {
		// Wait for the state container value to change.
		_, err = soStateCtr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return err
		}

		// Lock the writeMtx, so that we wait until any write txn is done processing first.
		lockCtx, lockTask := trace.NewTask(ctx, "alpha/watch-state/lock-write-mtx")
		unlockWriteMtx, err := c.writeMtx.Lock(lockCtx)
		lockTask.End()
		if err != nil {
			return err
		}

		// Separate lock acquisition from hold time so traces show contention vs work.
		holdCtx, holdTask := trace.NewTask(ctx, "alpha/watch-state/hold-write-mtx")

		// Get the latest snap in case it changed in the meantime.
		snap = soStateCtr.GetValue()

		// Watch the state once (sync any changes to soEngine and update local state).
		err = c.executeWatchSOStateOnce(holdCtx, snap, soEngine)

		// Be sure to unlock the writeMtx right away.
		holdTask.End()
		unlockWriteMtx()

		// Return the error, if any.
		if err != nil {
			return err
		}
	}
}

// executeWatchSOStateOnce processes a single shared object state change.
func (c *Controller) executeWatchSOStateOnce(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	soEngine *soEngine,
) error {
	// Trace the adoption.
	ctx, task := trace.NewTask(ctx, "alpha/watch-state/process-snapshot")
	defer task.End()

	// Only accepted roots may become the local World. Queue replay belongs to
	// the validator; installing its speculative result would make the next
	// transaction base disagree with this same SharedObject snapshot.
	state, err := snapshotWorldState(ctx, snap)
	if err != nil {
		return err
	}
	head := state.GetHeadRef()

	// Publish the accepted head and wake maintenance after successful adoption.
	taskCtx, task2 := trace.NewTask(ctx, "alpha/watch-state/update-engine-state")
	err = soEngine.updateEngineState(taskCtx, head)
	task2.End()
	if err == nil {
		c.notifyWrite()
	}
	return err
}
