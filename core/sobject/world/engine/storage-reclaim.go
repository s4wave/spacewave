package sobject_world_engine

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// storageReclaimDelay is the wait after a write before a storage reclaim pass,
// so the volume GC deletes the blocks the write released from the local store
// first.
const storageReclaimDelay = 5 * time.Minute

// storageReclaimInterval is the minimum time between storage reclaim passes.
// A pass reads the key index of every packfile on the storage backend.
const storageReclaimInterval = time.Hour

// errStorageReclaimNotReady is returned by the fence while the accepted World
// is not completely local.
var errStorageReclaimNotReady = errors.New("accepted World is not completely local")

// executeStorageReclaim runs a storage reclaim pass storageReclaimDelay after a
// write, and no sooner than storageReclaimInterval after the previous pass. It
// idles while the config leaves storage reclaim off.
func (c *Controller) executeStorageReclaim(ctx context.Context, so sobject.SharedObject) error {
	// Idle until shutdown while storage reclaim is off.
	if !c.conf.GetEnableStorageReclaim() {
		<-ctx.Done()
		return ctx.Err()
	}

	// Track the pending pass and the time of the previous one.
	var reclaimTimer *time.Timer
	var lastReclaim time.Time
	defer func() {
		if reclaimTimer != nil {
			reclaimTimer.Stop()
		}
	}()

	// Schedule a pass after each write, and run it when it is due. Hold each
	// wait channel until it fires, so a write during a pass is not missed.
	var waitCh <-chan struct{}
	for {
		if waitCh == nil {
			c.writeBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
				waitCh = getWaitCh()
			})
		}
		var reclaimCh <-chan time.Time
		if reclaimTimer != nil {
			reclaimCh = reclaimTimer.C
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
			waitCh = nil
			if reclaimTimer == nil {
				delay := max(storageReclaimDelay, time.Until(lastReclaim.Add(storageReclaimInterval)))
				reclaimTimer = time.NewTimer(delay)
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

// notifyWrite wakes the storage reclaim routine after a World change.
func (c *Controller) notifyWrite() {
	c.writeBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		broadcast()
	})
}

// reclaimStorage drops the blocks the local store no longer holds from the
// Space's storage backend. Only the validator or an owner runs a pass, because
// the pass advances the storage generation. A failed pass is logged, and the next write
// schedules another.
func (c *Controller) reclaimStorage(ctx context.Context, so sobject.SharedObject) error {
	// Only the validator or owner reclaims.
	canRun, err := c.isValidatorOrOwner(ctx, so)
	if err != nil || !canRun {
		return err
	}

	// Run the pass, logging a failure.
	err = so.GetBlockStore().ReclaimStorage(ctx, func(ctx context.Context) error {
		return c.advanceStorageGeneration(ctx, so)
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		c.le.WithError(err).Warn("storage reclaim pass failed")
	}
	return nil
}

// advanceStorageGeneration commits an AdvanceStorageGenerationOp, so every
// transaction built on the previous generation reruns and uploads its blocks
// again.
//
// The local store is the liveness test of the reclaim pass. While the accepted
// World is still copying into it, its missing blocks would look dead, so the
// fence returns errStorageReclaimNotReady instead.
func (c *Controller) advanceStorageGeneration(ctx context.Context, so sobject.SharedObject) error {
	rejected, err := c.commitMaintenanceOp(ctx, so, func(state *InnerState) (*SOWorldOp, error) {
		// Check the accepted World is completely local.
		complete, err := block.RootComplete(ctx, so.GetBlockStore(), state.GetHeadRef().GetRootRef())
		if err != nil {
			return nil, err
		}
		if !complete {
			return nil, errStorageReclaimNotReady
		}

		// Advance the generation the World was accepted on.
		return &SOWorldOp{
			Body: &SOWorldOp_AdvanceStorageGeneration{
				AdvanceStorageGeneration: &AdvanceStorageGenerationOp{
					StorageGeneration: state.GetStorageGeneration(),
				},
			},
		}, nil
	})
	if rejected {
		return errors.Wrap(err, "storage generation advance rejected")
	}
	return err
}

// isValidatorOrOwner reports whether the local participant is the validator or
// an owner, the roles allowed to run maintenance.
func (c *Controller) isValidatorOrOwner(ctx context.Context, so sobject.SharedObject) (bool, error) {
	// Read the local participant's role from the SharedObject state.
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
