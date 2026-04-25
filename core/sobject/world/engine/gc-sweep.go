package sobject_world_engine

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// gcSweepJournalThreshold is the default journal entry count that triggers a sweep.
const gcSweepJournalThreshold uint64 = 64

// gcSweepDefaultIdleWindow is the default idle window duration after the last write.
const gcSweepDefaultIdleWindow = 5 * time.Second

// gcSweepDefaultBackstopInterval is the default periodic backstop interval.
const gcSweepDefaultBackstopInterval = 5 * time.Minute

// executeGCSweepMaintenance runs the GC sweep maintenance routine.
// GC sweep queueing is gated on validator/owner role and re-checked on every
// attempted enqueue so role changes are picked up without restarting.
func (c *Controller) executeGCSweepMaintenance(ctx context.Context, so sobject.SharedObject, bengine *world_block.Engine) error {
	// Read configurable durations from the config proto.
	idleWindow := gcSweepDefaultIdleWindow
	if d := c.conf.GetGcSweepIdleWindowDur(); d != 0 {
		idleWindow = time.Duration(d)
	}
	backstopInterval := gcSweepDefaultBackstopInterval
	if d := c.conf.GetGcSweepBackstopIntervalDur(); d != 0 {
		backstopInterval = time.Duration(d)
	}

	c.le.Debug("gc sweep maintenance routine started")

	var idleTimer *time.Timer
	backstopTicker := time.NewTicker(backstopInterval)
	defer func() {
		backstopTicker.Stop()
		if idleTimer != nil {
			idleTimer.Stop()
		}
	}()

	for {
		var waitCh <-chan struct{}
		c.writeBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})

		var idleCh <-chan time.Time
		if idleTimer != nil {
			idleCh = idleTimer.C
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
			entries := bengine.GetGCJournalEntries()
			if entries >= gcSweepJournalThreshold {
				queued, err := c.queueGCSweepTx(ctx, so)
				if err != nil {
					return err
				}
				if queued {
					c.le.WithField("gc-journal-entries", entries).Debug("journal threshold exceeded, queued gc sweep")
					if idleTimer != nil {
						idleTimer.Stop()
						idleTimer = nil
					}
				}
				continue
			}

			if idleTimer == nil {
				idleTimer = time.NewTimer(idleWindow)
			} else {
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(idleWindow)
			}
		case <-idleCh:
			idleTimer = nil
			entries := bengine.GetGCJournalEntries()
			if entries > 0 {
				queued, err := c.queueGCSweepTx(ctx, so)
				if err != nil {
					return err
				}
				if queued {
					c.le.WithField("gc-journal-entries", entries).Debug("idle window expired with garbage, queued gc sweep")
				}
			}
		case <-backstopTicker.C:
			entries := bengine.GetGCJournalEntries()
			if entries > 0 {
				queued, err := c.queueGCSweepTx(ctx, so)
				if err != nil {
					return err
				}
				if queued {
					c.le.WithField("gc-journal-entries", entries).Debug("periodic backstop, queued gc sweep")
					if idleTimer != nil {
						idleTimer.Stop()
						idleTimer = nil
					}
				}
			}
		}
	}
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

// queueGCSweepTx constructs a GC_SWEEP transaction and queues it through
// SOWorldOp.ApplyTxOp. Returns whether a sweep was actually queued.
func (c *Controller) queueGCSweepTx(ctx context.Context, so sobject.SharedObject) (bool, error) {
	canQueue, err := c.canQueueGCSweepTx(ctx, so)
	if err != nil {
		return false, err
	}
	if !canQueue {
		return false, nil
	}

	tx, err := world_block_tx.NewTxGCSweep()
	if err != nil {
		return false, err
	}

	op := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}

	opData, err := op.MarshalVT()
	if err != nil {
		return false, err
	}

	localOpID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return false, err
	}

	c.le.WithField("op-id", localOpID).Debug("queued gc sweep tx")

	// Wait for the operation to be confirmed or rejected.
	_, rejected, err := so.WaitOperation(ctx, localOpID)
	if err != nil {
		return false, err
	}
	if rejected {
		c.le.Warn("gc sweep tx was rejected")
	}

	return true, nil
}
