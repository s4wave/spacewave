package sobject_world_engine

import (
	"context"
	"errors"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/peer"
)

// soEngineWriteTx holds the writer lock until its operation is placed or
// discarded.
type soEngineWriteTx struct {
	// WorldState records the operations applied to the fork.
	*world_block_tx.WorldState
	// btx contains the candidate World blocks.
	btx *world_block.Tx
	// eng adds the operation to the SharedObject operation set.
	eng *soEngine
	// fork is the replay position the write forked from.
	fork *replayFork
	// unlockWriteMtx releases the writer lock once, including repeated Discard calls.
	unlockWriteMtx func()
	// candidateRoot is the committed candidate World root whose staging
	// ownership Discard releases once the installed World holds it.
	candidateRoot *block.BlockRef
	// receipt is the durable publication of the write. It keeps the published
	// World pinned until Discard, after the engine installs it.
	receipt *block.PublicationReceipt
}

// newSoEngineWriteTx constructs a new shared object engine tx.
func newSoEngineWriteTx(
	worldState *world_block_tx.WorldState,
	btx *world_block.Tx,
	eng *soEngine,
	fork *replayFork,
	unlockWriteMtx func(),
) *soEngineWriteTx {
	return &soEngineWriteTx{
		WorldState:     worldState,
		btx:            btx,
		eng:            eng,
		fork:           fork,
		unlockWriteMtx: unlockWriteMtx,
	}
}

// Commit persists the candidate blocks, adds the transaction to the operation
// set and installs the World after it. When no other operation arrived since
// the fork, the candidate is that World; otherwise replay computes it. A
// SharedObject that publishes its state commits the blocks, the operation and
// the accepted World in one physical transaction.
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

	// Empty transactions have no operation to add.
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

	// The World bucket owns the candidate root until a publication transfers
	// it or Discard releases it, after the installed World holds it or replay
	// replaced it.
	t.candidateRoot = nroot

	// Without publication, write the blocks before the operation can enter
	// the set.
	publisher, publishes := t.eng.so.(sobject.StatePublisher)
	if !publishes {
		if err := t.writeBlocks(ctx); err != nil {
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

	// Wrap the batch in the SharedObject World operation.
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}).MarshalVT()
	if err != nil {
		return err
	}

	// The candidate is the World after the operation on the fork's position.
	next := t.fork.state.CloneVT()
	next.HeadRef = t.eng.bengine.GetRootRef()
	next.HeadRef.RootRef = nroot
	next.HeadRef.BucketId = ""
	t.fork.state = next
	if err := t.eng.c.retainPublicationWorld(ctx, t.eng.so, next.GetHeadRef()); err != nil {
		return err
	}

	// Every member replays the operation from the checkpoint and reads its
	// payloads, which the candidate World need not reference. Keep them owned
	// when Discard releases the blocks the candidate does not reach.
	t.btx.KeepRoots(t.TakePayloadRefs()...)

	// Add the operation and install the World after it.
	taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/queue-operation")
	defer task.End()
	if publishes {
		taskCtx = sobject.WithPublishState(taskCtx, func(ctx context.Context, head *block.AtomicHeadUpdate) (bool, error) {
			return t.publish(ctx, publisher, nroot, head)
		})
	}
	return t.eng.queueOperation(taskCtx, opData, t.fork)
}

// writeBlocks writes the candidate blocks. A state write ordered after the
// block writes needs them written, not flushed to the device.
func (t *soEngineWriteTx) writeBlocks(ctx context.Context) error {
	// Trace the block write.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/sync-blocks")
	defer task.End()

	// Flush the blocks when the queue orders the state after them.
	if sobject.QueueOrdersBlockWrites(t.eng.so) {
		return t.btx.Flush(ctx)
	}
	_, err := t.btx.Sync(ctx)
	return err
}

// publish commits the candidate blocks, the state head, the candidate root
// under the accepted World root and the replay save the last write deferred
// in one physical transaction, and reports true once it is durable. When
// head is nil or the store cannot publish, it writes the blocks and reports
// false; the SharedObject then commits its state itself.
func (t *soEngineWriteTx) publish(ctx context.Context, publisher sobject.StatePublisher, root *block.BlockRef, head *block.AtomicHeadUpdate) (bool, error) {
	// Fall back when the state or the store cannot join a publication.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/publish")
	defer task.End()
	store, ok := t.eng.so.GetBlockStore().(block.AtomicPublisher)
	if head == nil || !ok || !store.SupportsAtomicPublication() {
		return false, t.writeBlocks(ctx)
	}

	// Borrow the candidate blocks and collect the pending replay save.
	batch, err := t.btx.TakePending(ctx)
	if err != nil {
		return false, err
	}
	saveEntries, roots, cursor, err := t.eng.replay.pendingSave(publisher)
	if err != nil {
		batch.Complete(err)
		return false, err
	}
	heads := []*block.AtomicHeadUpdate{head}
	if cursor != nil {
		heads = append(heads, cursor)
	}

	// Submit them, writing the blocks alone when the store declines.
	receipt, err := store.SubmitAtomic(ctx, &block.AtomicPublication{
		Entries:  slices.Concat(batch.Entries, saveEntries),
		Heads:    heads,
		RootName: acceptedWorldRootName,
		Root:     root,
		Roots:    roots,
		Ordered:  world.OrderedCommit(ctx),
	})
	if errors.Is(err, block.ErrAtomicPublicationUnsupported) || errors.Is(err, block.ErrPublicationTooLarge) {
		batch.Complete(err)
		return false, t.writeBlocks(ctx)
	}
	if err != nil {
		batch.Complete(err)
		return false, err
	}

	// Admission commits the write to the durable result.
	err = receipt.Wait(context.WithoutCancel(ctx))
	batch.Complete(err)
	if err != nil {
		receipt.Release()
		return false, err
	}

	// The accepted World root holds the candidate, and the replay is saved.
	held := t.fork.state.GetHeadRef().CloneVT()
	held.BucketId = t.eng.so.GetBlockStore().GetID()
	t.receipt, t.candidateRoot, t.eng.retained = receipt, nil, held
	t.eng.replay.saved()
	t.fork.published = true
	return true, nil
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
// If called after Commit, releases the staging ownership of its candidate
// root. The payloads of its operations stay owned: replay from the checkpoint
// reads them again.
// Cannot return an error.
// Can be called unlimited times.
// Always call Discard or Commit when done with a tx.
func (t *soEngineWriteTx) Discard() {
	// Drop the candidate and let the next writer start.
	t.WorldState.Discard()
	t.btx.Discard()
	t.unlockWriteMtx()
	if t.receipt != nil {
		t.receipt.Release()
		t.receipt = nil
	}

	// The installed World holds the candidate or replay replaced it.
	if t.candidateRoot == nil {
		return
	}
	roots := []*block.BlockRef{t.candidateRoot}
	t.candidateRoot = nil
	if err := block.ReleaseRoots(context.Background(), t.eng.so.GetBlockStore(), roots); err != nil && t.eng.c != nil && t.eng.c.le != nil {
		t.eng.c.le.WithError(err).Warn("unable to release candidate root")
	}
}

// _ is a type assertion
var _ world.Tx = (*soEngineWriteTx)(nil)
