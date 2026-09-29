package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
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

	// Submit the operation and wait for authority's decision.
	localOpID, err := e.so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, err
	}
	acceptedSeqno, rejected, err := e.so.WaitOperation(ctx, localOpID)
	if err != nil && !rejected {
		return nil, err
	}

	// A storage generation advance rejects candidates built on the older
	// generation. The validator publishes the advance no later than the
	// rejection, so the snapshot shows it and the follower rebuilds the
	// candidate, uploading its blocks again.
	if rejected {
		_ = e.so.ClearOperationResult(ctx, localOpID)
		advanced, aerr := e.storageGenerationAdvanced(ctx, packet.GetOp())
		if aerr != nil {
			return nil, aerr
		}
		status := SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_REJECTED
		if advanced {
			status = SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE
		}
		return e.retainRejection(ctx, packet, status, err)
	}

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

// storageGenerationAdvanced reports whether the accepted storage generation is
// newer than the one op was built on.
func (e *soEngine) storageGenerationAdvanced(ctx context.Context, op *SOWorldOp) (bool, error) {
	// Only a transaction carries a storage generation.
	txOp := op.GetApplyTxOp()
	if txOp == nil {
		return false, nil
	}

	// Compare it with the accepted generation.
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return false, err
	}
	state, err := ReadInnerState(ctx, snap)
	if err != nil {
		return false, err
	}
	return state.GetStorageGeneration() > txOp.GetStorageGeneration(), nil
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
