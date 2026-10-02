package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// finalizeSpaceWorldCandidate submits a follower candidate to SharedObject
// authority and waits for its decision. A decision that does not accept the
// candidate retains it for follower-side cleanup.
func (e *soEngine) finalizeSpaceWorldCandidate(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
	opData []byte,
) (*SpaceWorldFinalizationDecision, error) {
	// Refuse a candidate authority cannot read or that is built on a stale base.
	if err := packet.Validate(); err != nil {
		return nil, err
	}
	if !packet.GetBlocksAvailable() {
		return e.retainRejection(
			ctx,
			packet,
			SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_MISSING_BLOCK,
			errors.New("candidate blocks unavailable to SharedObject authority"),
		)
	}
	if err := e.validateFinalizationBase(ctx, packet); err != nil {
		return e.retainRejection(ctx, packet, SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE, err)
	}

	// Submit the operation and wait for authority's decision. A validator that
	// finds a candidate block in no store rejects it as missing: store the
	// candidate's blocks again and resubmit once.
	for restored := false; ; restored = true {
		acceptedSeqno, rejected, err := e.submitFinalization(ctx, opData)
		if err != nil && !rejected {
			return nil, err
		}
		if !rejected {
			return e.acceptFinalization(ctx, packet, acceptedSeqno)
		}

		if !errors.Is(err, block.ErrNotFound) {
			return e.retainRejection(ctx, packet, SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_REJECTED, err)
		}

		// Restore the candidate's blocks once; a second miss stays missing.
		missing := SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_MISSING_BLOCK
		if restored {
			return e.retainRejection(ctx, packet, missing, err)
		}
		if rerr := e.restoreCandidateBlocks(ctx, packet); rerr != nil {
			return e.retainRejection(ctx, packet, missing, errors.Wrap(rerr, "restore candidate blocks"))
		}
	}
}

// submitFinalization queues opData and waits for authority's decision. It
// clears the result of a rejected operation.
func (e *soEngine) submitFinalization(ctx context.Context, opData []byte) (uint64, bool, error) {
	// Queue the operation and wait for its decision.
	localOpID, err := e.so.QueueOperation(ctx, opData)
	if err != nil {
		return 0, false, err
	}
	acceptedSeqno, rejected, err := e.so.WaitOperation(ctx, localOpID)

	// Clear the persisted rejection; the returned error carries it.
	if rejected {
		_ = e.so.ClearOperationResult(ctx, localOpID)
	}
	return acceptedSeqno, rejected, err
}

// uploadWaiter is a block store that uploads writes to a storage backend
// after they land locally.
type uploadWaiter interface {
	// WaitUploaded waits until every block written so far is uploaded.
	WaitUploaded(ctx context.Context) error
}

// restoreCandidateBlocks writes every block of the candidate World again, so
// a store that uploads its writes uploads them again, and waits for the
// upload. Recovery copies without completion proofs: the proofs say the blocks
// are local, not that the validator can read them.
func (e *soEngine) restoreCandidateBlocks(ctx context.Context, packet *SpaceWorldFinalizationPacket) error {
	// Write the candidate graph onto itself and make the writes durable.
	store := e.so.GetBlockStore()
	root := packet.GetCandidateWorldRoot().GetRootRef()
	if err := block.CopyGraph(ctx, store, store, root, nil); err != nil {
		return err
	}
	if _, err := store.Sync(ctx); err != nil {
		return err
	}

	// Wait for the storage backend to hold them.
	if w, ok := store.(uploadWaiter); ok {
		return w.WaitUploaded(ctx)
	}
	return nil
}

// acceptFinalization reports the accepted roots that include the packet's
// operation.
func (e *soEngine) acceptFinalization(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
	acceptedSeqno uint64,
) (*SpaceWorldFinalizationDecision, error) {
	// Report the accepted roots that include the operation.
	root, worldRoot, err := e.waitFinalizationAcceptedRoot(ctx, packet, acceptedSeqno)
	if err != nil {
		return nil, err
	}
	decision := &SpaceWorldFinalizationDecision{
		Status:                   SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_ACCEPTED,
		AcceptedSharedObjectRoot: root,
		AcceptedWorldRoot:        worldRoot,
		LocalOperationId:         packet.GetLocalOperationId(),
	}
	if err := decision.Validate(); err != nil {
		return nil, err
	}
	return decision, nil
}

// retainRejection decides status for packet because of cause and retains the
// candidate for follower-side cleanup. Every status but REJECTED is retryable.
func (e *soEngine) retainRejection(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
	status SpaceWorldFinalizationStatus,
	cause error,
) (*SpaceWorldFinalizationDecision, error) {
	decision := &SpaceWorldFinalizationDecision{
		Status:           status,
		Error:            cause.Error(),
		Retryable:        status != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_REJECTED,
		LocalOperationId: packet.GetLocalOperationId(),
	}
	if err := e.retainRejectedSpaceWorldCandidate(ctx, packet, decision); err != nil {
		return nil, err
	}
	return decision, nil
}

// waitFinalizationAcceptedRoot waits for the accepted root that includes the
// packet's operation and returns it with its World root.
func (e *soEngine) waitFinalizationAcceptedRoot(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
	acceptedSeqno uint64,
) (*sobject.SORoot, *bucket.ObjectRef, error) {
	// Return the current roots when they already include the operation.
	minSeqno := max(acceptedSeqno, packet.GetBaseSharedObjectRoot().GetInnerSeqno()+1)
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, nil, err
	}
	root, worldRoot, ok, err := finalizationSnapshotRoots(ctx, snap, minSeqno)
	if err != nil || ok {
		return root, worldRoot, err
	}

	// Otherwise watch the SharedObject state until they do.
	stateCtr, releaseStateCtr, err := e.so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	if releaseStateCtr != nil {
		defer releaseStateCtr()
	}
	if stateCtr == nil {
		return nil, nil, errors.New("SharedObject state watch is unavailable")
	}
	snap, err = stateCtr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		_, _, ok, err := finalizationSnapshotRoots(ctx, snap, minSeqno)
		return ok, err
	}, nil)
	if err != nil {
		return nil, nil, err
	}

	// Read the roots of the state that included it.
	root, worldRoot, ok, err = finalizationSnapshotRoots(ctx, snap, minSeqno)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, errors.New("accepted SharedObject root did not advance")
	}
	return root, worldRoot, nil
}

// finalizationSnapshotRoots returns the accepted roots of snap and whether its
// root reached minSeqno.
func finalizationSnapshotRoots(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	minSeqno uint64,
) (*sobject.SORoot, *bucket.ObjectRef, bool, error) {
	// Check the SharedObject root reached minSeqno.
	if snap == nil {
		return nil, nil, false, nil
	}
	root, err := snap.GetRootState(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	if root == nil || root.GetInnerSeqno() < minSeqno {
		return root, nil, false, nil
	}

	// Read the World root it accepted.
	state, err := snapshotWorldState(ctx, snap)
	if err != nil {
		return nil, nil, false, err
	}
	return root, state.GetHeadRef(), true, nil
}

// validateFinalizationBase checks the packet was built on the current accepted
// SharedObject and World roots.
func (e *soEngine) validateFinalizationBase(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
) error {
	// Compare the accepted SharedObject root.
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	root, err := snap.GetRootState(ctx)
	if err != nil {
		return err
	}
	if root == nil || packet.GetBaseSharedObjectRoot() == nil {
		return errors.New("base SharedObject root is missing")
	}
	if !root.EqualVT(packet.GetBaseSharedObjectRoot()) {
		return errors.New("base SharedObject root is stale")
	}

	// Compare the accepted World root.
	state, err := snapshotWorldState(ctx, snap)
	if err != nil {
		return err
	}
	if packet.GetBaseWorldRoot() == nil {
		return errors.New("base World root is missing")
	}
	if !state.GetHeadRef().GetRootRef().EqualsRef(packet.GetBaseWorldRoot().GetRootRef()) {
		return errors.New("base World root is stale")
	}
	return nil
}

// ReadInnerState parses the World state from a SharedObject snapshot.
// The state is empty until the World is initialized.
func ReadInnerState(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*InnerState, error) {
	// Decode the state data of the accepted root.
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, err
	}
	state := &InnerState{}
	if err := state.UnmarshalVT(rootInner.GetStateData()); err != nil {
		return nil, err
	}
	return state, nil
}

// snapshotWorldState parses the state of an initialized World from snap.
func snapshotWorldState(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*InnerState, error) {
	state, err := ReadInnerState(ctx, snap)
	if err != nil {
		return nil, err
	}
	if state.GetHeadRef() == nil {
		return nil, errors.New("World root head ref is missing")
	}
	return state, nil
}

// refreshFinalizationWorldRoot adopts the current accepted World root.
func (e *soEngine) refreshFinalizationWorldRoot(ctx context.Context) error {
	// Read the accepted World head and install it.
	snapshot, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	state, err := snapshotWorldState(ctx, snapshot)
	if err != nil {
		return err
	}
	return e.updateEngineState(ctx, state.GetHeadRef())
}
