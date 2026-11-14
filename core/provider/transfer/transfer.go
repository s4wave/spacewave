package provider_transfer

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
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
	copyIdentity   func(ctx context.Context) error

	bcast broadcast.Broadcast
	state *TransferState
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
		copyIdentity:   opts.CopyIdentity,
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

	// If specific space IDs were requested, filter to only those. Cleanup
	// deletes the source volume with every Space in it, so a transfer that
	// cleans up moves every Space or none.
	deletesSource := t.cleanup != nil &&
		(t.mode == TransferMode_TransferMode_MERGE || t.mode == TransferMode_TransferMode_MIGRATE)
	if len(t.filterSpaceIDs) > 0 {
		allowed := make(map[string]struct{}, len(t.filterSpaceIDs))
		for _, id := range t.filterSpaceIDs {
			allowed[id] = struct{}{}
		}
		scanned := len(entries)
		entries = slices.DeleteFunc(entries, func(e *sobject.SharedObjectListEntry) bool {
			_, ok := allowed[e.GetRef().GetProviderResourceRef().GetId()]
			return !ok
		})
		if left := scanned - len(entries); deletesSource && left != 0 {
			return t.fail(errors.Errorf("%s moves every space and deletes the source: %d spaces not selected", t.mode.String(), left))
		}
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

	// Phase: copying. Move each Space whole before the next, so a run that
	// stops leaves each Space checkpointed or for a retry to move again.
	t.setPhase(TransferPhase_TransferPhase_COPYING_BLOCKS)
	for i, entry := range entries {
		if _, ok := done[spaceIDs[i]]; ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return t.fail(err)
		}
		soID := spaceIDs[i]
		if err := t.moveSpace(ctx, i, entry); err != nil {
			return t.fail(errors.Wrapf(err, "transfer space %s", soID))
		}

		// Save checkpoint after each completed space.
		t.setSpacePhase(i, TransferPhase_TransferPhase_COMPLETE)
		done[soID] = struct{}{}
		t.saveCheckpoint(ctx, done)
		t.le.WithField("so-id", soID).Debug("space transfer complete")
	}

	// Give the target the identity its Spaces are signed with while the source
	// still holds it.
	if t.copyIdentity != nil {
		if err := t.copyIdentity(ctx); err != nil {
			return t.fail(errors.Wrap(err, "copy identity"))
		}
	}

	// Phase: cleanup source (MERGE and MIGRATE modes). Every Space the source
	// held, other than account-private ones, has reached the target and the
	// target holds the identity that signs them, so deleting the volume loses
	// only what the person chose to leave: account-private objects.
	if deletesSource {
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

// moveSpace copies the World a Space's source replays to the target, holds
// it there, and starts the target's lineage at it.
//
// Invariants, which the target's collector relies on to free nothing the
// moved Space needs (Transfer.tla checks them as StateHeld):
//
//  1. The copy's stage holds every copied block from its first put until
//     the import root holds the World's roots: the stage is released only by
//     the deferred release, after the hold is written and checked.
//  2. The hold is written after AddSharedObject, because adding an id the
//     target does not list releases every root of its bucket, as a create of
//     an abandoned id does. A retry finds the id listed, so the add releases
//     nothing and the hold appends to what an earlier run held.
//  3. The stage is checked after the hold: a run that stalled past the
//     stage's lease and the collector's grace may have lost the copy, and
//     the Space then fails before its state is written, so a retry copies
//     again.
//  4. The state is written only after the check, so the target never names
//     a World it does not hold. A Space is checkpointed only after its state
//     is written, and the source is cleaned up only after every Space is
//     checkpointed.
//  5. The hold releases nothing. A retained root the target's Adapter never
//     takes from the import root stays held until the Space is deleted.
//
// A target that holds no roots, such as a cloud block store, takes the copy
// with no stage and no hold. Such a copy is safe today only because Spacewave
// pack reclaim is off by default (SetPackReclaim). A reclaim pass trashes a
// pack at least half dead by the roots a fence reports, and this copy holds
// nothing a fence reports; a trashed pack stays readable, and a pass retires
// it only after packfile.TrashAge, rescuing first the blocks that are live
// again.
func (t *Transfer) moveSpace(ctx context.Context, idx int, entry *sobject.SharedObjectListEntry) error {
	// Name the Space for the target and the log.
	soRef := entry.GetRef()
	soID := soRef.GetProviderResourceRef().GetId()
	le := t.le.WithField("so-id", soID)

	// Replay the World and copy it to the target under a stage.
	t.setSpacePhase(idx, TransferPhase_TransferPhase_COPYING_BLOCKS)
	le.Debug("replaying and copying space")
	dst, releaseDst, err := t.target.GetBlockStore(ctx, soRef)
	if err != nil {
		return errors.Wrap(err, "mount target block store")
	}
	defer releaseDst()
	world, roots, stage, err := t.copyWorld(ctx, le, idx, soRef, dst)
	if err != nil {
		return err
	}
	if stage != nil {
		defer stage.Release()
	}

	// Add the Space to the target, then hold its World there and check the
	// stage held the copy until then.
	t.setSpacePhase(idx, TransferPhase_TransferPhase_COPYING_SO)
	if err := t.target.AddSharedObject(ctx, soRef, entry.GetMeta()); err != nil {
		return errors.Wrap(err, "add SO to target list")
	}
	if stage != nil {
		if err := space_world.HoldImport(ctx, dst, roots...); err != nil {
			return errors.Wrap(err, "hold copied World")
		}
		if err := stage.CheckHeld(ctx, roots...); err != nil {
			return errors.Wrap(err, "check copied World")
		}
	}

	// Start the target lineage at the replayed World.
	if err := t.target.WriteSharedObjectState(ctx, le, soID, t.owner, world); err != nil {
		return errors.Wrap(err, "write target SO state")
	}
	return nil
}

// copyWorld replays the World of the Space at soRef on the source and copies
// every block it reaches into dst, publishing the count. It returns the
// encoded World, its roots and the copy's stage, nil on a store that holds no
// roots. The replay holds the World on the source until the copy ends.
func (t *Transfer) copyWorld(
	ctx context.Context,
	le *logrus.Entry,
	idx int,
	soRef *sobject.SharedObjectRef,
	dst block.StoreOps,
) ([]byte, []*block.BlockRef, *block.StageCopy, error) {
	// Replay the World.
	world, releaseWorld, err := t.source.ReplaySharedObject(ctx, le, soRef)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "replay")
	}
	defer releaseWorld()

	// Encode the World and list the roots its state names.
	data, err := world.MarshalVT()
	if err != nil {
		return nil, nil, nil, err
	}
	var roots []*block.BlockRef
	if root := world.GetHeadRef().GetRootRef(); !root.GetEmpty() {
		roots = append(roots, root)
	}
	for _, retained := range world.GetRetainedRoots() {
		if root := retained.GetRootRef(); !root.GetEmpty() {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return data, nil, nil, nil
	}

	// Copy the World's graphs.
	src, releaseSrc, err := t.source.GetBlockStore(ctx, soRef)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "mount source block store")
	}
	defer releaseSrc()
	var copied uint64
	stage, err := provider_migration.CopyRoots(ctx, src, roots, dst, func(*block.BlockRef, []byte) {
		copied++
		t.setSpaceBlocksCopied(idx, copied)
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "copy blocks")
	}
	return data, roots, stage, nil
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
