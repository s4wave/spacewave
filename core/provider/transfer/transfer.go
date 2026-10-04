package provider_transfer

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
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
	owner          crypto.PrivKey
	filterSpaceIDs []string

	bcast broadcast.Broadcast
	state *TransferState
}

// TransferOptions carries the optional collaborators of a Transfer. The zero
// value is valid: no cleanup, no checkpointing, and no space-ID filter.
type TransferOptions struct {
	// Cleanup deletes the source volume after a successful transfer.
	Cleanup CleanupSource
	// Checkpoint persists resumable transfer progress.
	Checkpoint CheckpointStore
	// FilterSpaceIDs restricts the transfer to the listed Space IDs.
	FilterSpaceIDs []string
}

// NewTransfer constructs a Transfer for the given mode, source, target, and
// session indexes. Each transferred Space starts a new lineage on the target
// owned solely by owner, holding the World the source replayed.
func NewTransfer(
	le *logrus.Entry,
	mode TransferMode,
	source TransferSource,
	target TransferTarget,
	owner crypto.PrivKey,
	sourceSessionIdx, targetSessionIdx uint32,
	opts *TransferOptions,
) *Transfer {
	if opts == nil {
		opts = &TransferOptions{}
	}
	return &Transfer{
		le:             le,
		mode:           mode,
		source:         source,
		target:         target,
		cleanup:        opts.Cleanup,
		checkpoint:     opts.Checkpoint,
		owner:          owner,
		filterSpaceIDs: opts.FilterSpaceIDs,
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
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = t.state.CloneVT()
	})
	return state
}

// WatchState returns a state snapshot and wait channel from the same broadcast lock.
func (t *Transfer) WatchState() (*TransferState, <-chan struct{}) {
	var state *TransferState
	var ch <-chan struct{}
	t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		state = t.state.CloneVT()
		ch = getWaitCh()
	})
	return state, ch
}

// Fail marks the transfer failed and returns the failure error.
func (t *Transfer) Fail(err error) error {
	return t.fail(err)
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
// If a checkpoint from the same transfer exists, skips the spaces it completed.
func (t *Transfer) Execute(ctx context.Context) error {
	// Resume after the spaces a previous run completed.
	done := t.loadCompletedSpaces(ctx)

	// Phase: scanning
	t.setPhase(TransferPhase_TransferPhase_SCANNING)
	t.le.Info("scanning source shared objects")

	// List the source objects.
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
		if _, ok := done[soID]; ok {
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

	// Phase: replaying and copying blocks per space. Replay writes the blocks
	// of the replayed World, so it runs before the copy.
	t.setPhase(TransferPhase_TransferPhase_COPYING_BLOCKS)
	worlds := make([][]byte, len(entries))
	for i, entry := range entries {
		if _, ok := done[spaceIDs[i]]; ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return t.fail(err)
		}
		soRef := entry.GetRef()
		soID := soRef.GetProviderResourceRef().GetId()
		le := t.le.WithField("so-id", soID)

		t.setSpacePhase(i, TransferPhase_TransferPhase_COPYING_BLOCKS)
		le.Debug("replaying space")
		world, err := t.source.ReplaySharedObject(ctx, le, soRef)
		if err != nil {
			return t.fail(errors.Wrapf(err, "replay: %s", soID))
		}
		worlds[i], err = world.MarshalVT()
		if err != nil {
			return t.fail(err)
		}

		le.Debug("copying blocks for space")
		if err := t.copyBlocksForSpace(ctx, i, soRef); err != nil {
			return t.fail(errors.Wrapf(err, "copy blocks: %s", soID))
		}

		le.Debug("block copy complete for space")
	}

	// Phase: copying SO state and adding to target list.
	t.setPhase(TransferPhase_TransferPhase_COPYING_SO)
	for i, entry := range entries {
		if _, ok := done[spaceIDs[i]]; ok {
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

		// Start the target lineage at the replayed World.
		if err := t.target.WriteSharedObjectState(ctx, le, soID, t.owner, worlds[i]); err != nil {
			return t.fail(errors.Wrapf(err, "write target SO state: %s", soID))
		}

		t.setSpacePhase(i, TransferPhase_TransferPhase_COMPLETE)

		// Save checkpoint after each completed space.
		done[soID] = struct{}{}
		t.saveCheckpoint(ctx, done)
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

	// Clean up checkpoint on success. A failed delete leaves a stale complete
	// checkpoint that the next Execute resumes from as already-done, so log it.
	if t.checkpoint != nil {
		if err := t.checkpoint.DeleteCheckpoint(ctx); err != nil {
			t.le.WithError(err).Warn("failed to delete completed transfer checkpoint")
		}
	}

	// Report completion.
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

	// Skip block copying when the source has no block references.
	if len(blockRefs) == 0 {
		return nil
	}

	// Publish the total block count for this space.
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Spaces[spaceIdx].BlocksTotal = uint64(len(blockRefs))
		broadcast()
	})

	// Mount the source block store for reading blocks.
	srcBlocks, srcRel, err := t.source.GetBlockStore(ctx, soRef)
	if err != nil {
		return errors.Wrap(err, "mount source block store")
	}
	defer srcRel()

	// Mount the target block store for writing blocks.
	dstBlocks, dstRel, err := t.target.GetBlockStore(ctx, soRef)
	if err != nil {
		return errors.Wrap(err, "mount target block store")
	}
	defer dstRel()

	// Copy each block and publish its progress.
	var copied uint64
	for _, ref := range blockRefs {
		if err := ctx.Err(); err != nil {
			return t.fail(err)
		}

		// A source without refs (cloud packs) copies bytes only; the
		// transfer lists every block, so none are skipped.
		stored, err := srcBlocks.GetStoredBlock(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "read block %s", ref.MarshalString())
		}
		if stored == nil {
			continue
		}

		if _, _, err := dstBlocks.PutBlock(ctx, stored.Data, stored.PutOpts(ref)); err != nil {
			return errors.Wrapf(err, "write block %s", ref.MarshalString())
		}

		copied++
		t.setSpaceBlocksCopied(spaceIdx, copied)
	}

	// Flush the destination block store after copying.
	_, err = dstBlocks.Sync(ctx)
	return err
}

// loadCompletedSpaces returns the IDs of spaces a previous run of this same
// transfer completed. A checkpoint from a transfer with a different mode or
// session pair is ignored: its spaces were not copied to this target.
func (t *Transfer) loadCompletedSpaces(ctx context.Context) map[string]struct{} {
	// Start with no completed spaces when checkpointing is disabled.
	done := make(map[string]struct{})
	if t.checkpoint == nil {
		return done
	}

	// Load saved progress, falling back to a fresh transfer after read errors.
	cp, err := t.checkpoint.LoadCheckpoint(ctx)
	if err != nil {
		t.le.WithError(err).Warn("failed to load checkpoint, starting fresh")
		return done
	}

	// Ignore a checkpoint that has no transfer state.
	cpState := cp.GetState()
	if cpState == nil {
		return done
	}

	// Ignore checkpoint state from a different transfer.
	if cpState.GetMode() != t.state.GetMode() ||
		cpState.GetSourceSessionIndex() != t.state.GetSourceSessionIndex() ||
		cpState.GetTargetSessionIndex() != t.state.GetTargetSessionIndex() {
		t.le.Warn("ignoring checkpoint from a different transfer")
		return done
	}

	// Mark checkpointed spaces complete and return their IDs.
	ids := cp.GetSpaceIds()
	n := min(int(cp.GetCurrentSpaceIndex()), len(ids))
	for _, id := range ids[:n] {
		done[id] = struct{}{}
	}
	t.le.WithField("resume-count", len(done)).Info("resuming from checkpoint")
	return done
}

// saveCheckpoint persists the completed space IDs if a checkpoint store is set.
func (t *Transfer) saveCheckpoint(ctx context.Context, done map[string]struct{}) {
	if t.checkpoint == nil {
		return
	}
	spaceIDs := slices.Sorted(maps.Keys(done))
	cp := &TransferCheckpoint{
		State:             t.GetState(),
		SpaceIds:          spaceIDs,
		CurrentSpaceIndex: uint32(len(spaceIDs)), //nolint:gosec // bounded by the source shared object list length.
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
