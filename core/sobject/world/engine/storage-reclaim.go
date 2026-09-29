package sobject_world_engine

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// storageReclaimDelay is the wait after a GC sweep before a storage reclaim
// pass, so the volume GC deletes the swept blocks from the local store first.
const storageReclaimDelay = 5 * time.Minute

// storageReclaimInterval is the minimum time between storage reclaim passes.
// A pass reads the key index of every packfile on the storage backend.
const storageReclaimInterval = time.Hour

// errStorageReclaimNotReady is returned by the fence while the accepted World
// is not completely local.
var errStorageReclaimNotReady = errors.New("accepted World is not completely local")

// reclaimStorage drops the blocks the local store no longer holds from the
// Space's storage backend. Only the validator runs a pass, because only it can
// advance the storage generation. A failed pass is logged, and the next GC
// sweep schedules another.
func (c *Controller) reclaimStorage(ctx context.Context, so sobject.SharedObject) error {
	// Only the validator or owner reclaims.
	canRun, err := c.canQueueGCSweepTx(ctx, so)
	if err != nil || !canRun {
		return err
	}

	// Run the pass, logging a failure.
	err = so.GetBlockStore().ReclaimStorage(ctx, func(ctx context.Context) error {
		return c.advanceStorageGeneration(ctx, so)
	})
	if ctx.Err() != nil {
		return context.Canceled
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
