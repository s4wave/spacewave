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

// openStage is the in-process state of one stage.
type openStage struct {
	// written records that a transaction hung the stage under its process
	// owner, which may have committed.
	written bool
	// released records that the stage was released after a write, so the next
	// changing direct transaction removes its edge.
	released bool
}

// OpenStage registers a stage under the process owner of reader pins. Blocks
// written through the stage belong to it until a parent written outside the
// stage references them. The stage's first write hangs it under the owner in
// the same transaction, so a stage that writes nothing costs no transaction.
// The release leaves the stage's edge for the next changing direct
// transaction to remove, like a reader pin, and the sweep collects every block
// no other owner holds. Closing the volume releases its open stages, and
// ReapRootPins releases those of a crashed process.
func (v *Volume) OpenStage(ctx context.Context) (string, func(), error) {
	// Register the stage once the process owner holds its lease.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return "", nil, err
	}
	defer unlock()
	if _, err := v.rootPinOwnerLocked(ctx); err != nil {
		return "", nil, err
	}
	stage := stagePrefix + ulid.NewULID()
	v.stages[stage] = &openStage{}
	return stage, sync.OnceFunc(func() { v.releaseStage(stage) }), nil
}

// releaseStage forgets a stage that wrote nothing and marks a written stage
// for the next changing direct transaction to remove.
func (v *Volume) releaseStage(stage string) {
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	st := v.stages[stage]
	switch {
	case st == nil:
	case st.written:
		st.released = true
	default:
		delete(v.stages, stage)
	}
}

// useStage returns the process owner of an open stage, recording a write
// through it when write is set. It returns block.ErrStageReleased after the
// stage is released: a released stage's edges would hold blocks that nothing
// can reach or sweep.
func (v *Volume) useStage(ctx context.Context, stage string, write bool) (string, error) {
	// Find the stage under the Volume pin lock.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	st := v.stages[stage]
	if st == nil || st.released {
		return "", block.ErrStageReleased
	}
	if write {
		st.written = true
	}
	return v.rootPinOwner, nil
}

// claimStage hangs an open stage under its process owner in the transaction
// that writes through it, unless an earlier commit already did.
func (v *Volume) claimStage(ctx context.Context, rg *block_gc.RefGraph, stage string) error {
	// Record the write, then add the edges a committed write left out.
	owner, err := v.useStage(ctx, stage, true)
	if err != nil {
		return err
	}
	held, err := rg.HasIncomingRefs(ctx, stage)
	if err != nil || held {
		return err
	}
	return rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
		{Subject: block_gc.NodeGCRoot, Object: owner},
		{Subject: owner, Object: stage},
	}, nil)
}

// releasedStagesLocked returns the released stages, and appends to removes the
// edges a direct transaction removes to drop them. The caller holds rootPinMu.
func (v *Volume) releasedStagesLocked(removes []block_gc.RefEdge) ([]string, []block_gc.RefEdge) {
	var stages []string
	for stage, st := range v.stages {
		if st.released {
			stages = append(stages, stage)
			removes = append(removes, block_gc.RefEdge{Subject: v.rootPinOwner, Object: stage})
		}
	}
	return stages, removes
}

// PrepareStagedBlock writes a block owned by an open stage, together with the
// stage's ownership edge. It returns block.ErrStageReleased after the stage is
// released.
func (v *Volume) PrepareStagedBlock(ctx context.Context, stage string, data []byte, opts *block.PutOpts) (ref *block.BlockRef, existed bool, err error) {
	// Note a block without references, then write it under the stage.
	putOpts, _ := block.PutOptsWithoutSync(opts)
	v.noteUnknownRefs(putStaged, data, putOpts)
	err = v.prepareOwned(ctx, stage, len(data), v.claimStage, func(store block.StoreOps) error {
		ref, existed, err = store.PutBlock(ctx, data, putOpts)
		return err
	})

	// Count the block it stored.
	if err == nil {
		v.countPut(putStaged, len(data), existed, putOpts.GetRefsKnown())
	}
	return ref, existed, err
}

// PrepareStagedBlockBatch writes a batch owned by an open stage.
func (v *Volume) PrepareStagedBlockBatch(ctx context.Context, stage string, entries []*block.PutBatchEntry) (existed []bool, err error) {
	// An empty batch writes nothing.
	if len(entries) == 0 {
		return nil, nil
	}

	// Note blocks without references, then write the batch under the stage.
	v.noteUnknownRefsBatch(putStaged, entries)
	err = v.prepareOwned(ctx, stage, batchBytes(entries), v.claimStage, func(store block.StoreOps) error {
		existed, err = store.PutBlockBatch(ctx, entries)
		return err
	})

	// Count the blocks it stored.
	if err == nil {
		v.countPutBatch(putStaged, entries, existed)
	}
	return existed, err
}

// HoldStageCompleteRoots reports which roots have a completion proof, durable
// or pending, and makes the open stage own each of them in the same
// transaction. A sweep removes a proof together with its root, so a held root
// keeps the whole graph its proof names until the stage is released.
func (v *Volume) HoldStageCompleteRoots(ctx context.Context, stage string, refs []*block.BlockRef) ([]bool, error) {
	complete := make([]bool, len(refs))
	err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
		// Require the stage to be open.
		if _, err := v.useStage(ctx, stage, false); err != nil {
			return err
		}

		// Hold each root whose proof is in the graph or pending its write.
		var adds []block_gc.RefEdge
		for i, ref := range refs {
			if ref.GetEmpty() {
				continue
			}
			ok, err := v.rootCompleteTx(ctx, blocks, rg, ref)
			if err != nil {
				return err
			}
			if ok {
				complete[i] = true
				adds = append(adds, block_gc.RefEdge{Subject: stage, Object: block_gc.BlockIRI(ref)})
			}
		}
		if len(adds) == 0 {
			return nil
		}
		if err := v.claimStage(ctx, rg, stage); err != nil {
			return err
		}
		return rg.ApplyRefBatch(ctx, adds, nil)
	})
	if err != nil {
		return nil, err
	}
	return complete, nil
}
