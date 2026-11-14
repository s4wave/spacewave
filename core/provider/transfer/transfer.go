package provider_transfer

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
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
	deleteSource   func(ctx context.Context, keep func(context.Context) error) error
	checkpoint     CheckpointStore
	sessions       [2]*provider.ProviderResourceRef
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
		deleteSource:   opts.DeleteSource,
		checkpoint:     opts.Checkpoint,
		sessions:       [2]*provider.ProviderResourceRef{opts.SourceSession, opts.TargetSession},
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

// setSpaceNotMoved marks a space not moved for reason and broadcasts.
func (t *Transfer) setSpaceNotMoved(idx int, reason string) {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Spaces[idx].Phase = TransferPhase_TransferPhase_NOT_MOVED
		t.state.Spaces[idx].NotMovedReason = reason
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

// Not-moved reasons a transfer gives for a Space it leaves where it is.
const (
	reasonShared        = "shared Space; invite the target account"
	reasonSharedCloud   = "shared Space on a cloud account; moving it is not supported yet"
	reasonTargetHolds   = "the target holds a different version of this Space: created there, moved from another account, or changed since it was moved"
	reasonTargetChanged = "the Space changed on the target during the transfer"
	reasonNoOrigin      = "the cloud account holds an earlier copy of this Space or a different one, and cannot tell which yet; both copies stay as they are"
	reasonSourceMoving  = "the Space kept changing on the source during cleanup"
	reasonTargetLost    = "the target no longer holds the Space as it was moved"
	reasonOpsNotSent    = "operations not sent: the target has not received this account's latest changes to the shared Space"
)

// maxCleanupMoves bounds how often cleanup moves a Space again that changed
// on the source after it was moved.
const maxCleanupMoves = 3

// sendTimeout bounds how long cleanup waits for the target to receive the
// source's operations on a shared Space, the same bound the account delete
// that follows sets on its own stage. A variable only so a test can shorten
// it.
var sendTimeout = 2 * time.Minute

// Execute runs the transfer operation.
//
// The target is the only record of progress. Every run asks the target what
// it holds under each selected Space's id and acts on that alone
// (Transfer.tla checks the table):
//
//	Target holds                                Action
//	nothing                                     add, copy, hold, check, write
//	arriving from this source                   copy, hold, check, write
//	written from this source at its head        nothing; moved
//	written from this source, source moved on   copy, hold, check, write
//	the same shared Space, local source         nothing; moved
//	anything else                               not moved, with a reason
//
// Each write is a compare-and-set on the target state the run read, so an
// edit on the target between the read and the write leaves the Space not
// moved rather than overwritten. A run that leaves any Space not moved
// finishes with phase NOT_MOVED and cleans nothing up.
func (t *Transfer) Execute(ctx context.Context) error {
	// Record the run so the wizard resumes it after a restart.
	t.saveCheckpoint(ctx)

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
	spaces := make([]*SpaceTransferState, len(entries))
	for i, entry := range entries {
		spaces[i] = &SpaceTransferState{
			SharedObjectId: entry.GetRef().GetProviderResourceRef().GetId(),
			Meta:           entry.GetMeta().CloneVT(),
		}
	}
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.state.Spaces = spaces
		broadcast()
	})

	// Phase: copying. Move each Space whole before the next, so a run that
	// stops leaves each Space for the next run to find on the target.
	t.setPhase(TransferPhase_TransferPhase_COPYING_BLOCKS)
	heads := make([][]byte, len(entries))
	notMoved := 0
	for i, entry := range entries {
		if err := ctx.Err(); err != nil {
			return t.fail(err)
		}
		head, reason, err := t.moveSpace(ctx, i, entry)
		if err != nil {
			return t.fail(errors.Wrapf(err, "transfer space %s", spaces[i].GetSharedObjectId()))
		}
		if reason != "" {
			t.setSpaceNotMoved(i, reason)
			notMoved++
			continue
		}
		heads[i] = head
		t.setSpacePhase(i, TransferPhase_TransferPhase_COMPLETE)
	}

	// Clean up nothing when any Space stayed where it was.
	if notMoved != 0 {
		t.le.WithField("not-moved", notMoved).Warn("transfer finished with spaces not moved")
		return t.finish(ctx, TransferPhase_TransferPhase_NOT_MOVED)
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
	// target holds the identity that signs them, so deleting the account loses
	// only what the person chose to leave: account-private objects.
	if deletesSource {
		t.setPhase(TransferPhase_TransferPhase_CLEANUP)
		t.le.Info("cleaning up source after transfer")
		for i, entry := range entries {
			reason, err := t.cleanupSpace(ctx, i, entry, heads[i])
			if err != nil {
				return t.fail(errors.Wrapf(err, "clean up space %s", spaces[i].GetSharedObjectId()))
			}
			if reason != "" {
				t.setSpaceNotMoved(i, reason)
				return t.finish(ctx, TransferPhase_TransferPhase_NOT_MOVED)
			}
		}
		if t.deleteSource != nil {
			if err := t.deleteSource(ctx, t.sourceEmpty); err != nil {
				return t.fail(errors.Wrap(err, "delete source account"))
			}
		}
	}

	// Report completion.
	t.le.Info("transfer complete")
	return t.finish(ctx, TransferPhase_TransferPhase_COMPLETE)
}

// finish ends a run that reached its last phase: the wizard has nothing left
// to resume.
func (t *Transfer) finish(ctx context.Context, phase TransferPhase) error {
	if t.checkpoint != nil {
		if err := t.checkpoint.DeleteCheckpoint(ctx); err != nil {
			t.le.WithError(err).Warn("failed to delete finished transfer checkpoint")
		}
	}
	t.setPhase(phase)
	return nil
}

// cleanupSpace deletes the Space at idx from the source once the target
// holds it at the source's current head, which it checks just before the
// delete. A Space whose source moved on since it was moved is moved again,
// at most maxCleanupMoves times. It returns a reason when the Space must stay
// on the source.
//
// The delete also requires the target to hold the Space as moved, which it
// reads after the head check: the Space written at that head, or the shared
// Space's membership. A target deleted or changed since the move keeps the
// Space on the source; no lock elsewhere is trusted for that. A shared Space
// also needs every operation the source wrote in it on the target's copy,
// since the source's copy may hold the only one: see sharedSent.
//
// A write to the source between the head check and the delete is lost with
// the source; the source has no delete conditioned on its head yet.
func (t *Transfer) cleanupSpace(ctx context.Context, idx int, entry *sobject.SharedObjectListEntry, head []byte) (string, error) {
	// Name the Space for the checks and the log.
	soRef := entry.GetRef()
	soID := soRef.GetProviderResourceRef().GetId()
	le := t.le.WithField("so-id", soID)

	// A shared Space the target is a member of has no head of its own here:
	// the source's delete leaves it to its other members.
	if entry.GetSource() != "shared" {
		for moves := 0; ; moves++ {
			now, err := t.sourceHead(ctx, le, soRef)
			if err != nil {
				return "", err
			}
			if bytes.Equal(now, head) {
				break
			}
			if moves == maxCleanupMoves {
				return reasonSourceMoving, nil
			}
			le.Info("space changed on the source after it was moved, moving it again")
			var reason string
			head, reason, err = t.moveSpace(ctx, idx, entry)
			if err != nil || reason != "" {
				return reason, err
			}
			t.setSpacePhase(idx, TransferPhase_TransferPhase_COMPLETE)
		}
	}
	held, err := t.targetHolds(ctx, le, entry, head)
	if err != nil {
		return "", err
	}
	if !held {
		return reasonTargetLost, nil
	}
	if entry.GetSource() == "shared" {
		sent, err := t.sharedSent(ctx, le, soID)
		if err != nil {
			return "", err
		}
		if !sent {
			return reasonOpsNotSent, nil
		}
	}
	if err := t.cleanup.DeleteSharedObject(ctx, soID); err != nil {
		le.WithError(err).Warn("failed to delete source SO")
	}
	return "", nil
}

// sharedSent reports whether the target's copy of the shared Space soID
// holds every operation the source stores in it: the target's view of the
// source peer's chain reaches the source's head. It first asks the source to
// send what the target lacks and waits at most sendTimeout for the target to
// receive it. A source that wrote nothing in the Space needs no read.
//
// Invariant: the source deletes its copy of a shared Space only when that
// copy holds no operation the target lacks. Other members may hold the
// operations too, but the source cannot prove it, so it keeps them.
func (t *Transfer) sharedSent(ctx context.Context, le *logrus.Entry, soID string) (bool, error) {
	// Read the source's chain in the Space.
	peerID, nonce, err := t.cleanup.AuthorHead(ctx, soID)
	if err != nil {
		return false, errors.Wrap(err, "read the source's operations")
	}
	if nonce == 0 {
		return true, nil
	}

	// Send what the target lacks and wait for it to arrive.
	if !t.cleanup.SendSharedObject(soID) {
		le.Debug("no sync of the shared space is running to send its operations")
	}
	wctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	held, err := t.target.WaitAuthor(wctx, soID, peerID, nonce)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		held, err = false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "read the target's copy of the source's operations")
	}
	if !held {
		le.WithField("nonce", nonce).Warn("target lacks the source's operations on the shared space")
	}
	return held, nil
}

// sourceEmpty fails while the source lists a Space other than an
// account-private one. Cleanup deleted every Space the run moved, so such a
// Space was created or joined during the run, or its delete failed; deleting
// the volume would lose it.
func (t *Transfer) sourceEmpty(ctx context.Context) error {
	soList, err := t.source.GetSharedObjectList(ctx)
	if err != nil {
		return errors.Wrap(err, "list the source's spaces before its delete")
	}
	return ListsOnlyPrivate(soList)
}

// ListsOnlyPrivate fails when list holds a Space other than an
// account-private one. A transfer's source delete runs it, before its start
// and again when a resume finishes it, so the volume delete loses no Space the
// run did not move.
func ListsOnlyPrivate(list *sobject.SharedObjectList) error {
	for _, entry := range list.GetSharedObjects() {
		if !entry.GetMeta().GetAccountPrivate() {
			return errors.Errorf("the source still lists space %s, which the run did not move", entry.GetRef().GetProviderResourceRef().GetId())
		}
	}
	return nil
}

// targetHolds reports whether the target holds the Space of entry as the
// run moved it: a shared Space as a member, any other at the source World
// head names.
func (t *Transfer) targetHolds(ctx context.Context, le *logrus.Entry, entry *sobject.SharedObjectListEntry, head []byte) (bool, error) {
	// Ask the target about the Space at the head the run moved.
	soID := entry.GetRef().GetProviderResourceRef().GetId()
	at := t.source.GetOrigin().CloneVT()
	at.SourceHead = head
	tgt, err := t.target.ReadSharedObject(ctx, le, soID, at)
	if err != nil {
		return false, errors.Wrap(err, "read the target's space before cleanup")
	}

	// A shared Space needs the membership, any other the World at head.
	if entry.GetSource() == "shared" {
		return tgt.Holds == TargetHoldsShared, nil
	}
	return tgt.Holds == TargetHoldsWritten && bytes.Equal(tgt.Head, head), nil
}

// sourceHead returns the head of the source's Space at soRef.
func (t *Transfer) sourceHead(ctx context.Context, le *logrus.Entry, soRef *sobject.SharedObjectRef) ([]byte, error) {
	world, release, err := t.source.ReplaySharedObject(ctx, le, soRef)
	if err != nil {
		return nil, errors.Wrap(err, "replay")
	}
	defer release()
	return worldHead(world)
}

// moveSpace decides what to do with one Space from what the target holds
// under its id, as Execute's table says, and does it. It returns the source
// head the target holds when the Space is moved, or the reason it is not.
//
// Invariants, which the target's collector relies on to free nothing the
// moved Space needs (Transfer.tla checks them as StateHeld):
//
//  1. The copy's stage holds every copied block from its first put until
//     the import root holds the World's roots: the stage is released only by
//     the deferred release, after the hold is written and checked.
//  2. The copy and the hold follow AddSharedObject, because adding an id the
//     target does not list releases every root of its bucket, as a create of
//     an abandoned id does. A later run finds the id listed, so it adds
//     nothing and the hold appends to what an earlier run held.
//  3. The stage is checked after the hold: a run that stalled past the
//     stage's lease and the collector's grace may have lost the copy, and
//     the Space then fails before its state is written, so a retry copies
//     again.
//  4. The state is written only after the check, so the target never names
//     a World it does not hold, and only while the target holds the state
//     the run read (Transfer.tla: NoEditOverwritten).
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
func (t *Transfer) moveSpace(ctx context.Context, idx int, entry *sobject.SharedObjectListEntry) ([]byte, string, error) {
	// Name the Space and its source.
	soRef := entry.GetRef()
	soID := soRef.GetProviderResourceRef().GetId()
	le := t.le.WithField("so-id", soID)
	origin := t.source.GetOrigin()

	// A shared Space moves only as membership: the target account must be a
	// member already, and only a source whose delete leaves the Space to its
	// other members may count it moved.
	if entry.GetSource() == "shared" {
		tgt, err := t.target.ReadSharedObject(ctx, le, soID, origin)
		if err != nil {
			return nil, "", errors.Wrap(err, "read the target's space")
		}
		switch {
		case tgt.Holds == TargetHoldsNothing:
			return nil, reasonShared, nil
		case tgt.Holds != TargetHoldsShared:
			return nil, reasonTargetHolds, nil
		case !t.source.LeavesSharedObjects():
			return nil, reasonSharedCloud, nil
		}
		return nil, "", nil
	}

	// Replay the World and name it by its head.
	t.setSpacePhase(idx, TransferPhase_TransferPhase_COPYING_BLOCKS)
	world, releaseWorld, err := t.source.ReplaySharedObject(ctx, le, soRef)
	if err != nil {
		return nil, "", errors.Wrap(err, "replay")
	}
	defer releaseWorld()

	// Name the World by its head, and ask the target about that head.
	data, err := world.MarshalVT()
	if err != nil {
		return nil, "", err
	}
	head := encodedWorldHead(data)
	at := origin.CloneVT()
	at.SourceHead = head

	// Read what the target holds, and skip a Space it holds at the head.
	tgt, err := t.target.ReadSharedObject(ctx, le, soID, at)
	if err != nil {
		return nil, "", errors.Wrap(err, "read the target's space")
	}
	switch tgt.Holds {
	case TargetHoldsShared, TargetHoldsOther:
		return nil, reasonTargetHolds, nil
	case TargetHoldsNoOrigin:
		return nil, reasonNoOrigin, nil
	}
	if tgt.Holds == TargetHoldsWritten && bytes.Equal(tgt.Head, head) {
		le.Debug("target holds the space at the source's head, skipping")
		return head, "", nil
	}

	// List a Space the target does not list. A listing that appears since the
	// read is someone else's.
	if tgt.Holds == TargetHoldsNothing {
		tgt, err = t.target.AddSharedObject(ctx, soRef, entry.GetMeta(), origin)
		if errors.Is(err, sobject.ErrSharedObjectExists) {
			return nil, reasonTargetHolds, nil
		}
		if err != nil {
			return nil, "", errors.Wrap(err, "add SO to target list")
		}
	}

	// Copy the World to the target under a stage.
	le.Debug("copying space")
	dst, releaseDst, err := t.target.GetBlockStore(ctx, soRef)
	if err != nil {
		return nil, "", errors.Wrap(err, "mount target block store")
	}
	defer releaseDst()
	roots, stage, err := t.copyWorld(ctx, idx, soRef, world, dst)
	if err != nil {
		return nil, "", err
	}
	if stage != nil {
		defer stage.Release()
	}

	// Hold the World on the target and check the stage held the copy until
	// then.
	t.setSpacePhase(idx, TransferPhase_TransferPhase_COPYING_SO)
	if stage != nil {
		if err := space_world.HoldImport(ctx, dst, roots...); err != nil {
			return nil, "", errors.Wrap(err, "hold copied World")
		}
		if err := stage.CheckHeld(ctx, roots...); err != nil {
			return nil, "", errors.Wrap(err, "check copied World")
		}
	}

	// Start the target lineage at the replayed World, if the target still
	// holds what the run read.
	err = t.target.WriteSharedObjectState(ctx, le, soID, t.owner, data, at, tgt)
	if errors.Is(err, ErrTargetChanged) {
		return nil, reasonTargetChanged, nil
	}
	if err != nil {
		return nil, "", errors.Wrap(err, "write target SO state")
	}
	return head, "", nil
}

// copyWorld copies every block the replayed World reaches into dst,
// publishing the count. It returns the World's roots and the copy's stage,
// nil on a store that holds no roots or a World with none. The caller holds
// the replay until the copy ends.
func (t *Transfer) copyWorld(
	ctx context.Context,
	idx int,
	soRef *sobject.SharedObjectRef,
	world *space_world.InnerState,
	dst block.StoreOps,
) ([]*block.BlockRef, *block.StageCopy, error) {
	// List the roots the World's state names.
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
		return nil, nil, nil
	}

	// Copy the World's graphs.
	src, releaseSrc, err := t.source.GetBlockStore(ctx, soRef)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount source block store")
	}
	defer releaseSrc()
	var copied uint64
	stage, err := provider_migration.CopyRoots(ctx, src, roots, dst, func(*block.BlockRef, []byte) {
		copied++
		t.setSpaceBlocksCopied(idx, copied)
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "copy blocks")
	}
	return roots, stage, nil
}

// saveCheckpoint records the run's mode, sessions and selection for the
// wizard, if a checkpoint store is set. A failed save only loses the resume.
func (t *Transfer) saveCheckpoint(ctx context.Context) {
	if t.checkpoint == nil {
		return
	}
	err := t.checkpoint.SaveCheckpoint(ctx, &TransferCheckpoint{
		State:         t.GetState(),
		SpaceIds:      slices.Clone(t.filterSpaceIDs),
		SourceSession: t.sessions[0].CloneVT(),
		TargetSession: t.sessions[1].CloneVT(),
	})
	if err != nil {
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
