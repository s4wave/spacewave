package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	bifhash "github.com/s4wave/spacewave/net/hash"
)

// soEngineWriteTx holds the write mutex until its candidate is accepted or discarded.
type soEngineWriteTx struct {
	// WorldState records the operations applied to the fork.
	*world_block_tx.WorldState
	// btx contains the candidate World blocks.
	btx *world_block.Tx
	// eng publishes the candidate through SharedObject authority.
	eng *soEngine
	// baseRoot is the accepted SharedObject root captured before the write fork.
	baseRoot *sobject.SORoot
	// unlockWriteMtx releases the write mutex once, including repeated Discard calls.
	unlockWriteMtx func()
}

// newSoEngineWriteTx constructs a new shared object engine tx.
func newSoEngineWriteTx(
	worldState *world_block_tx.WorldState,
	btx *world_block.Tx,
	eng *soEngine,
	baseRoot *sobject.SORoot,
	unlockWriteMtx func(),
) *soEngineWriteTx {
	return &soEngineWriteTx{
		WorldState:     worldState,
		btx:            btx,
		eng:            eng,
		baseRoot:       baseRoot,
		unlockWriteMtx: unlockWriteMtx,
	}
}

// Commit persists candidate blocks and waits for SharedObject acceptance.
// A stale authority base rejects the candidate and refreshes the local World.
func (t *soEngineWriteTx) Commit(ctx context.Context) error {
	// Keep candidate cleanup and writer release bound to every return path.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/commit")
	defer task.End()
	defer t.Discard()

	// Close the operation buffer before publishing its candidate blocks.
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/world-state-commit")
		err := t.WorldState.Commit(taskCtx)
		task.End()
		if err != nil {
			return err
		}
	}

	// Empty transactions have no candidate to flush or submit to authority.
	txBatch := t.GetTxBatch()
	txns := txBatch.GetTxs()
	if len(txns) == 0 {
		return nil
	}

	// Commit every block generated for the candidate world root.
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

	// Fence pending block writes before the candidate root can enter the
	// SharedObject operation queue. A queue whose durable write orders the
	// block writes before it needs them written, not flushed to the device.
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/sync-blocks")
		var err error
		if sobject.QueueOrdersBlockWrites(t.eng.so) {
			err = t.btx.Flush(taskCtx)
		} else {
			_, err = t.btx.Sync(taskCtx)
		}
		task.End()
		if err != nil {
			return err
		}
	}

	// Serialize the complete mutation as one replayable transaction batch.
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

	// Wrap the batch in the SharedObject World operation.
	op := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}

	// Encode the operation for signing and validator replay.
	opData, err := op.MarshalVT()
	if err != nil {
		return err
	}

	// Keep the accepted base separate from the unpublished candidate root.
	baseObjRef := t.eng.bengine.GetRootRef() // clone of current (pre-commit) root
	nextObjRef := baseObjRef.CloneVT()
	nextObjRef.RootRef = nroot
	baseStoredObjRef := baseObjRef.CloneVT()
	baseStoredObjRef.BucketId = ""
	nextStoredObjRef := nextObjRef.CloneVT()
	nextStoredObjRef.BucketId = ""
	if err := t.eng.c.retainPublicationWorld(ctx, t.eng.so, nextStoredObjRef); err != nil {
		return err
	}

	// Bind the finalization packet to the encoded operation.
	contentID, err := bifhash.Sum(bifhash.HashType_HashType_SHA256, opData)
	if err != nil {
		return err
	}
	contentIDData, err := contentID.MarshalVT()
	if err != nil {
		return err
	}

	// Submit both bases captured for this write and its available candidate.
	candidateBlocksAvailable, err := t.eng.so.GetBlockStore().GetBlockExists(ctx, nextObjRef.GetRootRef())
	if err != nil {
		return err
	}
	packet := &SpaceWorldFinalizationPacket{
		BaseSharedObjectRoot:  t.baseRoot,
		BaseWorldRoot:         baseStoredObjRef,
		CandidateWorldRoot:    nextStoredObjRef,
		CandidateContentId:    contentIDData,
		BlocksAvailable:       candidateBlocksAvailable,
		Op:                    op,
		FollowerParticipantId: t.eng.so.GetPeerID().String(),
		LocalOperationId:      sobject.NewSOOperationLocalID(),
		StorageGeneration:     0,
		AuthorityEpoch:        t.baseRoot.GetInnerSeqno(),
	}

	// Cache the commit result for validator replay adoption. The validator
	// can adopt this instead of re-executing processOp when
	// the base root ref and op bytes match.
	{
		t.eng.c.lastCommitResult.Store(&commitResult{
			baseRootRef: baseObjRef.GetRootRef(),
			opData:      opData,
			resultState: &InnerState{HeadRef: nextStoredObjRef.CloneVT()},
		})
	}

	// Wait for authority without allowing the watcher to replace the write base.
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
	if err := finalizationDecisionError(decision); err != nil {
		if errors.Is(err, coord.ErrStaleGeneration) {
			if refreshErr := t.eng.refreshFinalizationWorldRoot(ctx); refreshErr != nil {
				return refreshErr
			}
		}
		return err
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

	// Wake maintenance only after the accepted World is visible locally.
	t.eng.c.notifyGCSweepMaintenance()
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

// finalizationDecisionError preserves the retryable stale-generation classification.
func finalizationDecisionError(decision *SpaceWorldFinalizationDecision) error {
	if decision.GetStatus() == SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_ACCEPTED {
		return nil
	}
	if decision.GetStatus() == SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE {
		return errors.Wrap(coord.ErrStaleGeneration, decision.GetError())
	}
	return errors.New(decision.GetError())
}

// _ is a type assertion
var _ world.Tx = (*soEngineWriteTx)(nil)
