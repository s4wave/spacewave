package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/bucket"
)

func (e *soEngine) finalizeSpaceWorldCandidate(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
	opData []byte,
) (*SpaceWorldFinalizationDecision, error) {
	if err := packet.Validate(); err != nil {
		return nil, err
	}
	if !packet.GetBlocksAvailable() {
		decision := &SpaceWorldFinalizationDecision{
			Status:           SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_MISSING_BLOCK,
			Error:            "candidate blocks unavailable to SharedObject authority",
			Retryable:        true,
			LocalOperationId: packet.GetLocalOperationId(),
		}
		if err := e.retainRejectedSpaceWorldCandidate(ctx, packet, decision); err != nil {
			return nil, err
		}
		return decision, nil
	}
	if err := e.validateFinalizationBase(ctx, packet); err != nil {
		decision := &SpaceWorldFinalizationDecision{
			Status:           SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE,
			Error:            err.Error(),
			Retryable:        true,
			LocalOperationId: packet.GetLocalOperationId(),
		}
		if err := e.retainRejectedSpaceWorldCandidate(ctx, packet, decision); err != nil {
			return nil, err
		}
		return decision, nil
	}

	localOpID, err := e.so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, err
	}
	acceptedSeqno, rejected, err := e.so.WaitOperation(ctx, localOpID)
	if err != nil {
		if rejected {
			_ = e.so.ClearOperationResult(ctx, localOpID)
			decision := &SpaceWorldFinalizationDecision{
				Status:           SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_REJECTED,
				Error:            err.Error(),
				LocalOperationId: packet.GetLocalOperationId(),
			}
			if err := e.retainRejectedSpaceWorldCandidate(ctx, packet, decision); err != nil {
				return nil, err
			}
			return decision, nil
		}
		return nil, err
	}

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

func (e *soEngine) waitFinalizationAcceptedRoot(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
	acceptedSeqno uint64,
) (*sobject.SORoot, *bucket.ObjectRef, error) {
	minSeqno := max(acceptedSeqno, packet.GetBaseSharedObjectRoot().GetInnerSeqno()+1)
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, nil, err
	}
	root, worldRoot, ok, err := finalizationSnapshotRoots(ctx, snap, minSeqno)
	if err != nil || ok {
		return root, worldRoot, err
	}

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
	root, worldRoot, ok, err = finalizationSnapshotRoots(ctx, snap, minSeqno)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, errors.New("accepted SharedObject root did not advance")
	}
	return root, worldRoot, nil
}

func finalizationSnapshotRoots(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	minSeqno uint64,
) (*sobject.SORoot, *bucket.ObjectRef, bool, error) {
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
	worldRoot, err := finalizationWorldRoot(ctx, snap)
	if err != nil {
		return nil, nil, false, err
	}
	return root, worldRoot, true, nil
}

func (e *soEngine) validateFinalizationBase(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
) error {
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
	worldRoot, err := finalizationWorldRoot(ctx, snap)
	if err != nil {
		return err
	}
	if worldRoot == nil || packet.GetBaseWorldRoot() == nil {
		return errors.New("base World root is missing")
	}
	if !worldRoot.GetRootRef().EqualsRef(packet.GetBaseWorldRoot().GetRootRef()) {
		return errors.New("base World root is stale")
	}
	return nil
}

func finalizationWorldRoot(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*bucket.ObjectRef, error) {
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, err
	}
	if rootInner == nil {
		return nil, errors.New("SharedObject root inner state is missing")
	}
	state := &InnerState{}
	if err := state.UnmarshalVT(rootInner.GetStateData()); err != nil {
		return nil, err
	}
	if state.GetHeadRef() == nil {
		return nil, errors.New("World root head ref is missing")
	}
	return state.GetHeadRef().Clone(), nil
}

func (e *soEngine) refreshFinalizationWorldRoot(ctx context.Context) error {
	snapshot, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	root, err := finalizationWorldRoot(ctx, snapshot)
	if err != nil {
		return err
	}
	return e.updateEngineState(ctx, root)
}
