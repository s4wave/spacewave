package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	bifhash "github.com/s4wave/spacewave/net/hash"
)

// soEngineWriteTx is the write txn attached to the soEngine
type soEngineWriteTx struct {
	*world_block_tx.WorldState
	btx            *world_block.Tx
	eng            *soEngine
	unlockWriteMtx func()
}

// newSoEngineWriteTx constructs a new shared object engine tx.
func newSoEngineWriteTx(
	worldState *world_block_tx.WorldState,
	btx *world_block.Tx,
	eng *soEngine,
	unlockWriteMtx func(),
) *soEngineWriteTx {
	return &soEngineWriteTx{
		WorldState:     worldState,
		btx:            btx,
		eng:            eng,
		unlockWriteMtx: unlockWriteMtx,
	}
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *soEngineWriteTx) Commit(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/commit")
	defer task.End()

	released := false
	release := func() {
		if released {
			return
		}
		released = true
		t.Discard()
	}
	defer release() // discard the underlying block txn and unlock the write mtx

	// commit the upper world state so we can get the txns list
	// world_block_tx.WorldState Commit just checks if discarded & marks as discarded
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/world-state-commit")
		err := t.WorldState.Commit(taskCtx)
		task.End()
		if err != nil {
			return err
		}
	}

	// commit the block txn
	var nroot *block.BlockRef
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/block-commit")
		var err error
		nroot, err = t.btx.CommitBlockTransaction(taskCtx)
		task.End()
		if err != nil {
			return err
		}
	}

	txBatch := t.GetTxBatch()
	txns := txBatch.GetTxs()
	if len(txns) == 0 {
		// no-op
		return nil
	}

	var tx *world_block_tx.Tx
	{
		_, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/build-tx-batch")
		var err error
		tx, err = world_block_tx.NewTxBatch(txBatch)
		task.End()
		if err != nil {
			return err
		}
	}

	// apply world op
	op := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}

	// marshal op data
	opData, err := op.MarshalVT()
	if err != nil {
		return err
	}

	// build the next obj ref
	baseObjRef := t.eng.bengine.GetRootRef() // clone of current (pre-commit) root
	nextObjRef := baseObjRef.CloneVT()
	nextObjRef.RootRef = nroot
	baseStoredObjRef := baseObjRef.CloneVT()
	baseStoredObjRef.BucketId = ""
	nextStoredObjRef := nextObjRef.CloneVT()
	nextStoredObjRef.BucketId = ""

	contentID, err := bifhash.Sum(bifhash.HashType_HashType_SHA256, opData)
	if err != nil {
		return err
	}
	contentIDData, err := contentID.MarshalVT()
	if err != nil {
		return err
	}

	baseSharedObjectState, err := t.eng.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	baseSharedObjectRoot, err := baseSharedObjectState.GetRootState(ctx)
	if err != nil {
		return err
	}
	if baseSharedObjectRoot == nil {
		return errors.New("base SharedObject root is missing")
	}
	candidateBlocksAvailable, err := t.eng.so.GetBlockStore().GetBlockExists(ctx, nextObjRef.GetRootRef())
	if err != nil {
		return err
	}
	packet := &SpaceWorldFinalizationPacket{
		BaseSharedObjectRoot:  baseSharedObjectRoot,
		BaseWorldRoot:         baseStoredObjRef,
		CandidateWorldRoot:    nextStoredObjRef,
		CandidateContentId:    contentIDData,
		BlocksAvailable:       candidateBlocksAvailable,
		Op:                    op,
		FollowerParticipantId: t.eng.so.GetPeerID().String(),
		LocalOperationId:      sobject.NewSOOperationLocalID(),
		StorageGeneration:     0,
		AuthorityEpoch:        baseSharedObjectRoot.GetInnerSeqno(),
	}

	// Cache the commit result for replay adoption. Watch-state and
	// validator can adopt this instead of re-executing processOp when
	// the base root ref and op bytes match.
	{
		t.eng.c.lastCommitResult.Store(&commitResult{
			baseRootRef: baseObjRef.GetRootRef(),
			opData:      opData,
			resultState: &InnerState{HeadRef: nextStoredObjRef.CloneVT()},
		})
	}

	var decision *SpaceWorldFinalizationDecision
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/finalize-candidate")
		var err error
		decision, err = t.eng.finalizeSpaceWorldCandidate(taskCtx, packet, opData)
		task.End()
		if err != nil {
			return err
		}
	}
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_ACCEPTED {
		return errors.New(decision.GetError())
	}

	// Update the local state only after SharedObject authority accepts the root.
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/update-engine-state")
		err := t.eng.updateEngineState(taskCtx, decision.GetAcceptedWorldRoot())
		task.End()
		if err != nil {
			return err
		}
	}

	t.eng.c.notifyGCSweepMaintenance()

	release()

	// done
	return nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
// Always call Discard or Commit when done with a tx.
func (t *soEngineWriteTx) Discard() {
	t.WorldState.Discard()
	t.btx.Discard()
	t.unlockWriteMtx()
}

// _ is a type assertion
var _ world.Tx = (*soEngineWriteTx)(nil)
