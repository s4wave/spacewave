package sobject_world_engine

import (
	"context"
	"errors"

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

// executeWatchSOStateOnce replays one shared object state into the World.
func (c *Controller) executeWatchSOStateOnce(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	soEngine *soEngine,
) error {
	ctx, task := trace.NewTask(ctx, "alpha/watch-state/process-snapshot")
	defer task.End()
	_, err := soEngine.advance(ctx, snap)
	return err
}

// isReadAccessLoss reports whether err means this participant cannot replay
// the state, which a later grant or readmission can restore.
func isReadAccessLoss(err error) bool {
	return errors.Is(err, sobject.ErrCannotDecode) ||
		errors.Is(err, sobject.ErrNotParticipant) ||
		errors.Is(err, sobject.ErrKeyEpochUnavailable) ||
		errors.Is(err, sobject.ErrConfigHistoryUnavailable)
}

// waitReadableSnapshot waits for a snapshot this participant can replay: it
// decodes the checkpoint and holds the config and key of every operation.
// Other snapshot errors end the wait so the caller surfaces them.
func waitReadableSnapshot(ctx context.Context, soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot]) error {
	_, err := soStateCtr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		return !isReadAccessLoss(checkReplayable(ctx, snap)), nil
	}, nil)
	return err
}

// checkReplayable returns the first error replaying snap would meet reading
// its checkpoint, configs and keys.
func checkReplayable(ctx context.Context, snap sobject.SharedObjectStateSnapshot) error {
	// The participant must decode the checkpoint.
	if _, err := snap.GetParticipantConfig(ctx); err != nil {
		return err
	}
	if _, err := snap.GetCheckpoint(ctx); err != nil {
		return err
	}

	// Every operation needs its config and its epoch key.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return err
	}
	for _, h := range set.Order() {
		inner := set.Get(h)
		if _, err := snap.GetConfigByHash(ctx, inner.GetConfigHash()); err != nil {
			return err
		}
		if _, err := snap.DecodeOperation(ctx, inner); err != nil && isReadAccessLoss(err) {
			return err
		}
	}
	return nil
}
