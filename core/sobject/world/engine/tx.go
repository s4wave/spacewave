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
	"github.com/s4wave/spacewave/net/peer"
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
	// storageGeneration is the accepted storage generation captured with baseRoot.
	storageGeneration uint64
	// unlockWriteMtx releases the write mutex once, including repeated Discard calls.
	unlockWriteMtx func()
	// candidateRoot is the committed candidate World root whose staging
	// ownership Discard releases. Nil while the outcome is unknown and while
	// an accepted candidate waits for the accepted head to hold it.
	candidateRoot *block.BlockRef
	// settledRoots are earlier unsettled candidates this write's acceptance
	// settled. Discard releases their staging ownership.
	settledRoots []*block.BlockRef
}

// newSoEngineWriteTx constructs a new shared object engine tx.
func newSoEngineWriteTx(
	worldState *world_block_tx.WorldState,
	btx *world_block.Tx,
	eng *soEngine,
	baseRoot *sobject.SORoot,
	storageGeneration uint64,
	unlockWriteMtx func(),
) *soEngineWriteTx {
	return &soEngineWriteTx{
		WorldState:        worldState,
		btx:               btx,
		eng:               eng,
		baseRoot:          baseRoot,
		storageGeneration: storageGeneration,
		unlockWriteMtx:    unlockWriteMtx,
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

	// The World bucket owns the candidate root until Discard releases it. An
	// accepted root moves under the accepted-world head instead.
	t.candidateRoot = nroot

	// Fence pending block writes before the candidate root can enter the
	// SharedObject operation queue. A queue write ordered after the block
	// writes needs them written, not flushed to the device.
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
	// Object roots in this participant's bucket replay into the bucket of the
	// replaying participant, whose World then owns their blocks.
	txBatch.ClearBucketID(t.eng.so.GetBlockStore().GetID())
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

	// Wrap the batch in the SharedObject World operation. The storage
	// generation fences the candidate against bucket reclamation.
	op := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx, StorageGeneration: t.storageGeneration},
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
		AuthorityEpoch:        t.baseRoot.GetInnerSeqno(),
	}

	// Cache the commit result for validator replay adoption. The validator
	// can adopt this instead of re-executing processOp when the base root
	// ref, storage generation, and op bytes match.
	t.eng.c.lastCommitResult.Store(&commitResult{
		baseRootRef:       baseObjRef.GetRootRef(),
		storageGeneration: t.storageGeneration,
		opData:            opData,
		resultRef:         nextStoredObjRef.CloneVT(),
	})

	// Wait for authority without allowing the watcher to replace the write base.
	// An ordered world commit lets the provider accept its operation ordered.
	var decision *SpaceWorldFinalizationDecision
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/finalize-candidate")
		if world.OrderedCommit(taskCtx) {
			taskCtx = sobject.WithOrderedOperation(taskCtx)
		}
		var err error
		decision, err = t.eng.finalizeSpaceWorldCandidate(taskCtx, packet, opData)
		task.End()
		if err != nil {
			// The authority may still accept the queued candidate, so it stays
			// staged until a later candidate is accepted.
			t.eng.unsettled = append(t.eng.unsettled, nroot)
			t.candidateRoot = nil
			return err
		}
	}

	// Refresh the base after a stale rejection.
	if err := finalizationDecisionError(decision); err != nil {
		if errors.Is(err, coord.ErrStaleGeneration) {
			if refreshErr := t.eng.refreshFinalizationWorldRoot(ctx); refreshErr != nil {
				return refreshErr
			}
		}
		return err
	}

	// Update the local state only after SharedObject authority accepts the
	// root. An accepted candidate stays owned until the accepted head holds it.
	if decision.GetAcceptedWorldRoot().GetRootRef().EqualVT(nroot) {
		t.candidateRoot = nil
	}
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/update-engine-state")
		err := t.eng.updateEngineState(taskCtx, decision.GetAcceptedWorldRoot())
		task.End()
		if err != nil {
			return err
		}
	}

	// The accepted head holds the World now, so release the candidate's
	// staging ownership. A write that left the World unchanged commits the
	// current head again, and installing an unchanged head keeps that edge.
	// Earlier unsettled candidates were ordered before this one, so the
	// authority can no longer accept them.
	t.candidateRoot = nroot
	t.settledRoots, t.eng.unsettled = t.eng.unsettled, nil

	// Wake maintenance only after the accepted World is visible locally.
	t.eng.c.notifyWrite()
	return nil
}

// ApplyWorldOp applies op to the candidate as accepted replay will apply it.
// Replay attributes an authenticated operation to the verified signer, so the
// candidate runs it as this engine's device with the device's accepted person.
func (t *soEngineWriteTx) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	if _, authenticated := op.(world.AuthenticatedOperation); authenticated {
		device, person, err := t.eng.OperationAuthor(ctx)
		if err != nil {
			return 0, false, err
		}
		sender = device
		ctx = world.WithOperationPerson(ctx, person)
	}
	return t.WorldState.ApplyWorldOp(ctx, op, sender)
}

// Discard cancels the transaction.
// If called after Commit, releases the payloads of its operations and the
// staging ownership of its candidate root.
// Cannot return an error.
// Can be called unlimited times.
// Always call Discard or Commit when done with a tx.
func (t *soEngineWriteTx) Discard() {
	// Drop the candidate and let the next writer start.
	t.WorldState.Discard()
	t.btx.Discard()
	t.unlockWriteMtx()

	// Commit has returned, so authority has accepted or rejected the
	// operations and no replay needs their payloads. The accepted World keeps
	// any payload it references.
	roots := append(t.TakePayloadRefs(), t.settledRoots...)
	t.settledRoots = nil
	if t.candidateRoot != nil {
		roots = append(roots, t.candidateRoot)
		t.candidateRoot = nil
	}
	if err := block.ReleaseRoots(context.Background(), t.eng.so.GetBlockStore(), roots); err != nil && t.eng.c != nil && t.eng.c.le != nil {
		t.eng.c.le.WithError(err).Warn("unable to release operation payloads and candidate root")
	}
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
