package kvtx

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// stagePrefix prefixes the node that owns the blocks written through one stage.
const stagePrefix = "stage:"

// OpenStage creates a stage node under the process owner of reader pins. Blocks
// written through the stage belong to it until a parent written outside the
// stage references them. The release removes the stage, and the sweep collects
// every block no other owner holds. Closing the volume releases its open
// stages, and ReapRootPins releases those of a crashed process.
func (v *Volume) OpenStage(ctx context.Context) (string, func(), error) {
	// Reserve the process owner under its lease.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return "", nil, err
	}
	owner, err := v.rootPinOwnerLocked(ctx)
	unlock()
	if err != nil {
		return "", nil, err
	}

	// Hang the stage under the process owner.
	stage := stagePrefix + ulid.NewULID()
	err = v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
		return rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
			{Subject: block_gc.NodeGCRoot, Object: owner},
			{Subject: owner, Object: stage},
		}, nil)
	})
	if err != nil {
		return "", nil, err
	}

	// A failed release leaves the stage until the volume closes or its owner
	// is reaped.
	release := sync.OnceFunc(func() {
		ctx := context.Background()
		_ = v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
			if _, err := rg.RemoveNodeRefs(ctx, stage, true); err != nil {
				return err
			}
			return rg.RemoveRef(ctx, owner, stage)
		})
	})
	return stage, release, nil
}

// PrepareStagedBlock writes a block owned by an open stage, together with the
// stage's ownership edge. It returns block.ErrStageReleased after the stage is
// released.
func (v *Volume) PrepareStagedBlock(ctx context.Context, stage string, data []byte, opts *block.PutOpts) (ref *block.BlockRef, existed bool, err error) {
	putOpts, _ := block.PutOptsWithoutSync(opts)
	err = v.prepareOwned(ctx, stage, claimStage, func(store block.StoreOps) error {
		ref, existed, err = store.PutBlock(ctx, data, putOpts)
		return err
	})
	return ref, existed, err
}

// PrepareStagedBlockBatch writes a batch owned by an open stage.
func (v *Volume) PrepareStagedBlockBatch(ctx context.Context, stage string, entries []*block.PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return v.prepareOwned(ctx, stage, claimStage, func(store block.StoreOps) error {
		return store.PutBlockBatch(ctx, entries)
	})
}

// claimStage requires the stage to still hang under its process owner. A
// released stage's edges would hold blocks that nothing can reach or sweep.
func claimStage(ctx context.Context, rg *block_gc.RefGraph, stage string) error {
	held, err := rg.HasIncomingRefs(ctx, stage)
	if err != nil {
		return err
	}
	if !held {
		return block.ErrStageReleased
	}
	return nil
}
