package sobject_world_engine

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// gcSweepJournalThreshold is the default journal entry count that triggers a sweep.
const gcSweepJournalThreshold uint64 = 64

// gcSweepDefaultIdleWindow is the default idle window duration after the last write.
const gcSweepDefaultIdleWindow = 5 * time.Second

// gcSweepDefaultBackstopInterval is the default periodic backstop interval.
const gcSweepDefaultBackstopInterval = 5 * time.Minute

type gcJournalEntryCounter interface {
	// GetGCJournalEntries returns the number of pending GC journal entries.
	GetGCJournalEntries() uint64
}

// executeGCSweepMaintenance runs the GC sweep maintenance routine.
// GC sweep queueing is gated on validator/owner role and re-checked on every
// attempted enqueue so role changes are picked up without restarting.
//
// Each queued sweep schedules a storage reclaim pass storageReclaimDelay
// later, and no sooner than storageReclaimInterval after the previous pass.
func (c *Controller) executeGCSweepMaintenance(ctx context.Context, so sobject.SharedObject, bengine gcJournalEntryCounter) error {
	// Idle until shutdown while maintenance is disabled.
	if c.gcSweepMaintenanceDisabled() {
		c.le.Debug("gc sweep maintenance disabled")
		<-ctx.Done()
		return ctx.Err()
	}

	// Read configurable durations from the config proto.
	idleWindow := gcSweepDefaultIdleWindow
	if d := c.conf.GetGcSweepIdleWindowDur(); d != 0 {
		idleWindow = time.Duration(d) //nolint:gosec // configuration duration is already represented in nanoseconds.
	}
	backstopInterval := gcSweepDefaultBackstopInterval
	if d := c.conf.GetGcSweepBackstopIntervalDur(); d != 0 {
		backstopInterval = time.Duration(d) //nolint:gosec // configuration duration is already represented in nanoseconds.
	}
	c.le.Debug("gc sweep maintenance routine started")

	// Track the idle window, the backstop, and the pending reclaim pass.
	var idleTimer, reclaimTimer *time.Timer
	var lastReclaim time.Time
	backstopTicker := time.NewTicker(backstopInterval)
	defer func() {
		backstopTicker.Stop()
		stopTimer(idleTimer)
		stopTimer(reclaimTimer)
	}()

	// sweep queues a GC sweep and, when one was queued, schedules a reclaim
	// pass and drops the pending idle expiry.
	sweep := func(reason string, entries uint64) error {
		// Queue the sweep.
		queued, err := c.queueGCSweepTx(ctx, so)
		if err != nil || !queued {
			return err
		}
		c.le.WithField("gc-journal-entries", entries).Debug(reason + ", queued gc sweep")

		// End the idle window and schedule a reclaim pass.
		stopTimer(idleTimer)
		idleTimer = nil
		if reclaimTimer == nil {
			delay := max(storageReclaimDelay, time.Until(lastReclaim.Add(storageReclaimInterval)))
			reclaimTimer = time.NewTimer(delay)
		}
		return nil
	}

	// Sweep on write bursts, idle expiry, and the backstop, and reclaim
	// storage when a pass is due.
	for {
		var waitCh <-chan struct{}
		c.writeBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})

		var idleCh, reclaimCh <-chan time.Time
		if idleTimer != nil {
			idleCh = idleTimer.C
		}
		if reclaimTimer != nil {
			reclaimCh = reclaimTimer.C
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
			entries := bengine.GetGCJournalEntries()
			if entries >= gcSweepJournalThreshold {
				if err := sweep("journal threshold exceeded", entries); err != nil {
					return err
				}
				continue
			}

			stopTimer(idleTimer)
			idleTimer = time.NewTimer(idleWindow)
		case <-idleCh:
			idleTimer = nil
			entries := bengine.GetGCJournalEntries()
			// Idle expiry is only a latency shortcut for threshold-sized garbage.
			// Sparse journal entries wait for the backstop so small write bursts do
			// not queue a GC sweep after every idle window.
			if entries >= gcSweepJournalThreshold {
				if err := sweep("idle window expired with threshold garbage", entries); err != nil {
					return err
				}
			}
		case <-backstopTicker.C:
			if entries := bengine.GetGCJournalEntries(); entries > 0 {
				if err := sweep("periodic backstop", entries); err != nil {
					return err
				}
			}
		case <-reclaimCh:
			reclaimTimer = nil
			lastReclaim = time.Now()
			if err := c.reclaimStorage(ctx, so); err != nil {
				return err
			}
		}
	}
}

// stopTimer stops timer when it is set.
func stopTimer(timer *time.Timer) {
	if timer != nil {
		timer.Stop()
	}
}

// gcSweepMaintenanceDisabled reports whether the config disables both sweep
// triggers.
func (c *Controller) gcSweepMaintenanceDisabled() bool {
	return c.conf.GetGcSweepIdleWindowDur() == 0 &&
		c.conf.GetGcSweepBackstopIntervalDur() == 0
}

// notifyGCSweepMaintenance wakes the GC sweep maintenance routine after a
// world-state change that may have produced pending GC journal entries.
func (c *Controller) notifyGCSweepMaintenance() {
	c.writeBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		broadcast()
	})
}

// canQueueGCSweepTx checks if the local participant is allowed to enqueue GC
// sweep maintenance transactions.
func (c *Controller) canQueueGCSweepTx(ctx context.Context, so sobject.SharedObject) (bool, error) {
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return false, err
	}

	participant, err := snap.GetParticipantConfig(ctx)
	if err != nil {
		return false, err
	}

	return sobject.IsValidatorOrOwner(participant.GetRole()), nil
}

// queueGCSweepTx queues a GC_SWEEP transaction when this participant is the
// validator or owner, and waits for its decision. Returns whether a sweep was
// queued.
func (c *Controller) queueGCSweepTx(ctx context.Context, so sobject.SharedObject) (bool, error) {
	// Only the validator or owner sweeps.
	canQueue, err := c.canQueueGCSweepTx(ctx, so)
	if err != nil || !canQueue {
		return false, err
	}

	// Sweep on the accepted storage generation. A rejected sweep was still
	// queued.
	rejected, err := c.commitMaintenanceOp(ctx, so, func(state *InnerState) (*SOWorldOp, error) {
		tx, err := world_block_tx.NewMaintenanceTxGCSweep()
		if err != nil {
			return nil, err
		}
		return &SOWorldOp{
			Body: &SOWorldOp_ApplyTxOp{
				ApplyTxOp: &ApplyTxOp{Tx: tx, StorageGeneration: state.GetStorageGeneration()},
			},
		}, nil
	})
	if rejected {
		c.le.WithError(err).Warn("gc sweep tx was rejected")
		return true, nil
	}
	return err == nil, err
}

// commitMaintenanceOp builds an operation on the accepted World state, queues
// it, and waits for the validator's decision. A rejection is cleared from the
// SharedObject state and returned as rejected with its error.
//
// The operation advances the SharedObject root like a foreground write, so it
// holds writeMtx from building until the decision. Otherwise a write
// transaction open across it would commit against a stale base.
func (c *Controller) commitMaintenanceOp(
	ctx context.Context,
	so sobject.SharedObject,
	build func(state *InnerState) (*SOWorldOp, error),
) (bool, error) {
	// Exclude write transactions until the decision.
	unlockWriteMtx, err := c.writeMtx.Lock(ctx)
	if err != nil {
		return false, err
	}
	defer unlockWriteMtx()

	// Build the operation on the accepted World state.
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return false, err
	}
	state, err := ReadInnerState(ctx, snap)
	if err != nil {
		return false, err
	}
	op, err := build(state)
	if err != nil {
		return false, err
	}

	// Queue it and wait for the decision.
	opData, err := op.MarshalVT()
	if err != nil {
		return false, err
	}
	localOpID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return false, err
	}
	_, rejected, err := so.WaitOperation(ctx, localOpID)
	if rejected {
		_ = so.ClearOperationResult(ctx, localOpID)
	}
	return rejected, err
}
