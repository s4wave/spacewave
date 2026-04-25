package provider_transfer

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// Transfer orchestrates a transfer operation between source and target accounts.
type Transfer struct {
	le             *logrus.Entry
	mode           TransferMode
	source         TransferSource
	target         TransferTarget
	cleanup        CleanupSource
	checkpoint     CheckpointStore
	stateRewriter  SOStateRewriter
	filterSpaceIDs []string

	bcast broadcast.Broadcast
	state *TransferState
}

// NewTransfer creates a new Transfer.
// cleanup may be nil to skip source cleanup after merge.
// checkpoint may be nil to disable checkpoint persistence.
// stateRewriter may be nil to copy SO state verbatim (only safe when source
// and target share the same peer key).
// filterSpaceIDs, if non-empty, restricts the transfer to only those space IDs.
func NewTransfer(
	le *logrus.Entry,
	mode TransferMode,
	source TransferSource,
	target TransferTarget,
	sourceSessionIdx, targetSessionIdx uint32,
	cleanup CleanupSource,
	checkpoint CheckpointStore,
	stateRewriter SOStateRewriter,
	filterSpaceIDs []string,
) *Transfer {
	return &Transfer{
		le:             le,
		mode:           mode,
		source:         source,
		target:         target,
		cleanup:        cleanup,
		checkpoint:     checkpoint,
		stateRewriter:  stateRewriter,
		filterSpaceIDs: filterSpaceIDs,
		state: &TransferState{
			Mode:               mode,
			Phase:              TransferPhase_TransferPhase_IDLE,
			SourceSessionIndex: sourceSessionIdx,
			TargetSessionIndex: targetSessionIdx,
		},
	}
}

// GetState returns a snapshot of the current transfer state.
func (t *Transfer) GetState() *TransferState {
	var state *TransferState
	t.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		state = t.state.CloneVT()
	})
	return state
}

// Fail marks the transfer failed and returns the failure error.
func (t *Transfer) Fail(err error) error {
	return t.fail(err)
}

// WaitState returns the wait channel for state changes.
func (t *Transfer) WaitState() <-chan struct{} {
	var ch <-chan struct{}
	t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	return ch
}

// setPhase updates the overall phase and broadcasts.
func (t *Transfer) setPhase(phase TransferPhase) {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Phase = phase
		broadcast()
	})
}

// setSpacePhase updates a space's phase and broadcasts.
func (t *Transfer) setSpacePhase(idx int, phase TransferPhase) {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Spaces[idx].Phase = phase
		broadcast()
	})
}

// setSpaceBlocksCopied updates the blocks copied count for a space.
func (t *Transfer) setSpaceBlocksCopied(idx int, count uint64) {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Spaces[idx].BlocksCopied = count
		broadcast()
	})
}

// Execute runs the transfer operation.
// If a checkpoint exists, resumes from the last saved position.
func (t *Transfer) Execute(ctx context.Context) error {
	// Try to load checkpoint for resume.
	var resumeIdx uint32
	if t.checkpoint != nil {
		cp, err := t.checkpoint.LoadCheckpoint(ctx)
		if err != nil {
			t.le.WithError(err).Warn("failed to load checkpoint, starting fresh")
		}
		if cp != nil && cp.GetState() != nil {
			resumeIdx = cp.GetCurrentSpaceIndex()
			t.le.WithField("resume-idx", resumeIdx).Info("resuming from checkpoint")
		}
	}

	// Phase: scanning
	t.setPhase(TransferPhase_TransferPhase_SCANNING)
	t.le.Info("scanning source shared objects")

	soList, err := t.source.GetSharedObjectList(ctx)
	if err != nil {
		return t.fail(errors.Wrap(err, "scan source SO list"))
	}

	// Filter out account-private SOs that are per-account and should not be
	// transferred between accounts.
	entries := slices.DeleteFunc(slices.Clone(soList.GetSharedObjects()), func(e *sobject.SharedObjectListEntry) bool {
		return e.GetMeta().GetAccountPrivate()
	})

	// If specific space IDs were requested, filter to only those.
	if len(t.filterSpaceIDs) > 0 {
		allowed := make(map[string]struct{}, len(t.filterSpaceIDs))
		for _, id := range t.filterSpaceIDs {
			allowed[id] = struct{}{}
		}
		entries = slices.DeleteFunc(entries, func(e *sobject.SharedObjectListEntry) bool {
			_, ok := allowed[e.GetRef().GetProviderResourceRef().GetId()]
			return !ok
		})
	}
	t.le.WithField("count", len(entries)).Info("found shared objects to transfer")

	// Initialize per-space state.
	spaceIDs := make([]string, len(entries))
	spaces := make([]*SpaceTransferState, len(entries))
	for i, entry := range entries {
		soID := entry.GetRef().GetProviderResourceRef().GetId()
		spaceIDs[i] = soID
		phase := TransferPhase_TransferPhase_IDLE
		if uint32(i) < resumeIdx {
			phase = TransferPhase_TransferPhase_COMPLETE
		}
		spaces[i] = &SpaceTransferState{
			SharedObjectId: soID,
			Phase:          phase,
			Meta:           entry.GetMeta().CloneVT(),
		}
	}
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Spaces = spaces
		broadcast()
	})

	// Phase: copying blocks per space.
	t.setPhase(TransferPhase_TransferPhase_COPYING_BLOCKS)
	for i, entry := range entries {
		if uint32(i) < resumeIdx {
			continue
		}
		if err := ctx.Err(); err != nil {
			return t.fail(err)
		}
		soRef := entry.GetRef()
		soID := soRef.GetProviderResourceRef().GetId()
		le := t.le.WithField("so-id", soID)

		t.setSpacePhase(i, TransferPhase_TransferPhase_COPYING_BLOCKS)
		le.Debug("copying blocks for space")

		if err := t.copyBlocksForSpace(ctx, i, soRef); err != nil {
			return t.fail(errors.Wrapf(err, "copy blocks: %s", soID))
		}

		le.Debug("block copy complete for space")
	}

	// Phase: copying SO state and adding to target list.
	t.setPhase(TransferPhase_TransferPhase_COPYING_SO)
	for i, entry := range entries {
		if uint32(i) < resumeIdx {
			continue
		}
		if err := ctx.Err(); err != nil {
			return t.fail(err)
		}
		soRef := entry.GetRef()
		soID := soRef.GetProviderResourceRef().GetId()
		meta := entry.GetMeta()
		le := t.le.WithField("so-id", soID)

		t.setSpacePhase(i, TransferPhase_TransferPhase_COPYING_SO)
		le.Debug("copying SO state and adding to target list")

		// Add the SO to the target before writing state so targets that enforce
		// resource existence and RBAC on state writes can accept the update.
		if err := t.target.AddSharedObject(ctx, soRef, meta); err != nil {
			return t.fail(errors.Wrapf(err, "add SO to target list: %s", soID))
		}

		// Copy SO state from source to target object store if it exists.
		// The state may not exist if the SO was created but never mounted.
		state, err := t.source.GetSharedObjectState(ctx, soID)
		if err != nil && !errors.Is(err, sobject.ErrSharedObjectNotFound) {
			return t.fail(errors.Wrapf(err, "read source SO state: %s", soID))
		}
		if state != nil {
			// Re-key the state for the target peer if a rewriter is configured.
			if t.stateRewriter != nil {
				state, err = t.stateRewriter(ctx, soID, state)
				if err != nil {
					return t.fail(errors.Wrapf(err, "re-key SO state: %s", soID))
				}
			}
			if err := t.target.WriteSharedObjectState(ctx, soID, state); err != nil {
				return t.fail(errors.Wrapf(err, "write target SO state: %s", soID))
			}
		}

		t.setSpacePhase(i, TransferPhase_TransferPhase_COMPLETE)

		// Save checkpoint after each completed space.
		t.saveCheckpoint(ctx, spaceIDs, uint32(i+1))
		le.Debug("SO merge complete for space")
	}

	// Phase: cleanup source (MERGE and MIGRATE modes).
	if (t.mode == TransferMode_TransferMode_MERGE || t.mode == TransferMode_TransferMode_MIGRATE) && t.cleanup != nil {
		t.setPhase(TransferPhase_TransferPhase_CLEANUP)
		t.le.Info("cleaning up source after transfer")

		for _, entry := range entries {
			soID := entry.GetRef().GetProviderResourceRef().GetId()
			if err := t.cleanup.DeleteSharedObject(ctx, soID); err != nil {
				t.le.WithError(err).WithField("so-id", soID).Warn("failed to delete source SO")
			}
		}

		if err := t.cleanup.DeleteVolume(ctx); err != nil {
			t.le.WithError(err).Warn("failed to delete source volume")
		}
	}

	// Clean up checkpoint on success.
	if t.checkpoint != nil {
		_ = t.checkpoint.DeleteCheckpoint(ctx)
	}

	t.setPhase(TransferPhase_TransferPhase_COMPLETE)
	t.le.Info("transfer complete")
	return nil
}

// copyBlocksForSpace copies all blocks for a single SO from source to target.
func (t *Transfer) copyBlocksForSpace(ctx context.Context, spaceIdx int, soRef *sobject.SharedObjectRef) error {
	// Get block refs from the source's GC ref graph.
	blockRefs, err := t.source.GetBlockRefs(ctx, soRef)
	if err != nil {
		return errors.Wrap(err, "get block refs")
	}

	if len(blockRefs) == 0 {
		return nil
	}

	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Spaces[spaceIdx].BlocksTotal = uint64(len(blockRefs))
		broadcast()
	})

	srcBlocks, srcRel, err := t.source.GetBlockStore(ctx, soRef)
	if err != nil {
		return errors.Wrap(err, "mount source block store")
	}
	defer srcRel()

	dstBlocks, dstRel, err := t.target.GetBlockStore(ctx, soRef)
	if err != nil {
		return errors.Wrap(err, "mount target block store")
	}
	defer dstRel()

	var copied uint64
	for _, ref := range blockRefs {
		if err := ctx.Err(); err != nil {
			return t.fail(err)
		}

		data, found, err := srcBlocks.GetBlock(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "read block %s", ref.MarshalString())
		}
		if !found {
			continue
		}

		if _, _, err := dstBlocks.PutBlock(ctx, data, nil); err != nil {
			return errors.Wrapf(err, "write block %s", ref.MarshalString())
		}

		copied++
		t.setSpaceBlocksCopied(spaceIdx, copied)
	}

	return nil
}

// saveCheckpoint persists the current progress if a checkpoint store is set.
func (t *Transfer) saveCheckpoint(ctx context.Context, spaceIDs []string, nextIdx uint32) {
	if t.checkpoint == nil {
		return
	}
	cp := &TransferCheckpoint{
		State:             t.GetState(),
		SpaceIds:          spaceIDs,
		CurrentSpaceIndex: nextIdx,
	}
	if err := t.checkpoint.SaveCheckpoint(ctx, cp); err != nil {
		t.le.WithError(err).Warn("failed to save checkpoint")
	}
}

// fail sets the transfer to failed state and returns the error.
func (t *Transfer) fail(err error) error {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Phase = TransferPhase_TransferPhase_FAILED
		t.state.ErrorMessage = err.Error()
		broadcast()
	})
	return err
}
