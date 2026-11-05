package sobject_world_engine

import (
	"context"
	"errors"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// executeWatchSOState watches and processes shared object state changes and
// settles the receipts of admitted writes as they resolve.
func (c *Controller) executeWatchSOState(
	ctx context.Context,
	soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	soEngine *soEngine,
) error {
	// Retain the inputs captured under the writer after each replay pass.
	var snap sobject.SharedObjectStateSnapshot
	var gap replayGap
	var settled <-chan struct{}

	// Follow every state and receipt notification.
	for {
		// Wait for the state to change, the input replay stopped for to
		// arrive, or the oldest pending receipt to resolve.
		_, err := c.waitReplayInput(ctx, soEngine.so, soStateCtr, snap, gap, settled)
		if err != nil {
			return err
		}

		// Lock the writeMtx, so that we wait until any write txn is done processing first.
		lockCtx, lockTask := trace.NewTask(ctx, "alpha/watch-state/lock-write-mtx")
		unlockWriteMtx, err := c.writeMtx.Lock(lockCtx, "watch-state")
		lockTask.End()
		if err != nil {
			return err
		}

		// Separate lock acquisition from hold time so traces show contention vs work.
		holdCtx, holdTask := trace.NewTask(ctx, "alpha/watch-state/hold-write-mtx")

		// Get the latest snap in case it changed in the meantime, settle the
		// resolved receipts, and skip a snapshot of a failed publication.
		snap = soStateCtr.GetValue()
		soEngine.settle()
		stale, err := c.stale(holdCtx, soEngine.so, snap)

		// Watch the state once (sync any changes to soEngine and update local state).
		if err == nil && !stale {
			err = c.executeWatchSOStateOnce(holdCtx, snap, soEngine)
		}
		gap = soEngine.replay.gap
		settled = soEngine.settled()

		// Be sure to unlock the writeMtx right away.
		holdTask.End()
		unlockWriteMtx()

		// Return the error, if any.
		if err != nil {
			return err
		}
	}
}

// waitReplayInput waits for the state to change from snap and returns the new
// state. When gap is set, it also returns the current state once the input it
// names arrives, since replay stopped for it. It also returns the current
// state once settled closes.
func (c *Controller) waitReplayInput(
	ctx context.Context,
	so sobject.SharedObject,
	soStateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	snap sobject.SharedObjectStateSnapshot,
	gap replayGap,
	settled <-chan struct{},
) (sobject.SharedObjectStateSnapshot, error) {
	// Without another input only a state change resumes replay.
	if gap.empty() && settled == nil {
		return soStateCtr.WaitValueChange(ctx, snap, nil)
	}

	// End the state wait once a receipt settles.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	notified := make(chan struct{})
	if settled != nil {
		go func() {
			defer close(notified)
			select {
			case <-settled:
				cancel()
			case <-waitCtx.Done():
			}
		}()
	} else {
		close(notified)
	}

	// Wait for the input replay stopped for, ending the state wait once it
	// arrives.
	fetched := make(chan struct{})
	go func() {
		// Skip the wait when replay stopped for no input.
		defer close(fetched)
		if gap.empty() {
			return
		}

		// Wait for the input, ending the state wait once it arrives.
		err := c.waitGap(waitCtx, so, gap)
		if err != nil && waitCtx.Err() == nil && !errors.Is(err, block.ErrNotFound) {
			c.le.WithError(err).Warn("wait for the input replay stopped for")
		}
		if err == nil {
			cancel()
		}
	}()

	// Join both waits before inspecting the next snapshot. An old notifier
	// must not consume an encoding edge after the next pass checked its state.
	next, err := soStateCtr.WaitValueChange(waitCtx, snap, nil)
	cancel()
	<-fetched
	<-notified

	// Another input ends the wait with the current state.
	if err != nil && ctx.Err() == nil && errors.Is(err, context.Canceled) {
		return soStateCtr.GetValue(), nil
	}
	return next, err
}

// executeWatchSOStateOnce replays one shared object state into the World. A
// replay gap, a block that is not available or a declared handler that has
// not resolved, stops replay at its operation without ending the watch: the
// World before that operation stays served, and the next state change, write,
// or arrival of the input resumes replay there.
func (c *Controller) executeWatchSOStateOnce(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	soEngine *soEngine,
) error {
	// Trace the pass.
	ctx, task := trace.NewTask(ctx, "alpha/watch-state/process-snapshot")
	defer task.End()

	// Replay, waiting out a gap.
	_, err := soEngine.advance(ctx, snap, nil)
	if isReplayGap(err) {
		return nil
	}
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
