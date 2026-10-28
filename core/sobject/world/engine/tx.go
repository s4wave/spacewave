package sobject_world_engine

import (
	"context"
	"errors"
	fmt "fmt"
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
	// batch borrows the candidate blocks into the publication until its
	// receipt resolves.
	batch *block.PendingBatch
	// clock times the stages of the write.
	clock *commitClock
}

// newSoEngineWriteTx constructs a new shared object engine tx.
func newSoEngineWriteTx(
	worldState *world_block_tx.WorldState,
	btx *world_block.Tx,
	eng *soEngine,
	fork *replayFork,
	unlockWriteMtx func(),
	clock *commitClock,
) *soEngineWriteTx {
	return &soEngineWriteTx{
		WorldState:     worldState,
		btx:            btx,
		eng:            eng,
		fork:           fork,
		unlockWriteMtx: unlockWriteMtx,
		clock:          clock,
	}
}

// Commit persists the candidate blocks, adds the transaction to the operation
// set and installs the World after it. When no other operation arrived since
// the fork, the candidate is that World; otherwise replay computes it. A
// SharedObject that publishes its state commits the blocks, the operation and
// the accepted World in one physical transaction: Commit releases the writer
// lock once the publication admits the write and the engine installs its
// World, so the next write forks from it, then waits for the receipt and
// returns its error. A write is acknowledged only after its receipt.
func (t *soEngineWriteTx) Commit(ctx context.Context) error {
	// Keep candidate cleanup and writer release bound to every return path.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/commit")
	defer task.End()
	defer t.Discard()
	defer t.clock.finish(t.eng.c.le)
	t.clock.end(stageCallback)

	// Close the operation buffer before publishing its candidate blocks.
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/world-state-commit")
		err := t.WorldState.Commit(taskCtx)
		task.End()
		t.clock.end(stageWorldStateCommit)
		if err != nil {
			return fmt.Errorf("world state commit: %w", err)
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
		t.clock.end(stageBlockCommit)
		if err != nil {
			return fmt.Errorf("world block commit: %w", err)
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
			return fmt.Errorf("world block store write: %w", err)
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
	// It derives from the fork's head, which the engine installed when the
	// fork began, because the engine's head is gone once the engine closes.
	next := t.fork.state.CloneVT()
	next.HeadRef.RootRef = nroot
	next.HeadRef.BucketId = ""
	t.fork.state = next
	if err := t.eng.c.retainPublicationWorld(ctx, t.eng.so, next.GetHeadRef()); err != nil {
		return fmt.Errorf("retain published World: %w", err)
	}

	// Every member replays the operation from the checkpoint and reads its
	// payloads, which the candidate World need not reference. The replay span
	// holds them from the publication, or once replay places the operation,
	// before Discard releases the blocks the candidate does not reach.
	t.fork.payloads = t.TakePayloadRefs()

	// Add the operation and install the World after it.
	taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/queue-operation")
	defer task.End()
	if publishes {
		taskCtx = sobject.WithPublishState(taskCtx, func(ctx context.Context, head *block.AtomicHeadUpdate) (*block.PublicationReceipt, error) {
			return t.publish(ctx, publisher, nroot, head)
		})
	}
	err = t.eng.queueOperation(taskCtx, opData, t.fork, t.clock)
	t.clock.end(stageAdvance)

	// Wait for the publication outside the writer lock. Its failure is the
	// write's result, since queueOperation may report only what followed it.
	t.unlockWriteMtx()
	if err != nil {
		err = fmt.Errorf("queue World operation: %w", err)
	}
	if t.receipt == nil {
		return err
	}
	if rerr := t.waitReceipt(ctx); rerr != nil {
		return fmt.Errorf("world publication receipt: %w", rerr)
	}
	return err
}

// waitReceipt waits for the publication to resolve, finishes the borrow of the
// candidate blocks with its result and returns its error, once the engine
// reinstalled the World from before a failed publication. Admission commits
// the write to the durable result, so the wait ignores ctx's cancellation and
// ends with the engine's lifetime. The outcome is then unknown: waitReceipt
// returns the wait's error, and the publisher's result finishes the borrow and
// releases the receipt once it resolves.
func (t *soEngineWriteTx) waitReceipt(ctx context.Context) error {
	// Wait for the receipt or the engine's lifetime.
	defer t.clock.end(stageReceiptWait)
	receipt, batch := t.receipt, t.batch
	waitCtx, cancel := t.eng.detach(ctx)
	defer cancel()
	err := receipt.Wait(waitCtx)
	select {
	case <-receipt.Done():
	default:
		t.receipt = nil
		go func() {
			<-receipt.Done()
			batch.Complete(receipt.Err())
			receipt.Release()
		}()
		return err
	}

	// A failed publication returns only after the engine reinstalls the World
	// from before it. The accepted World root holds a published candidate.
	err = receipt.Err()
	batch.Complete(err)
	if err != nil {
		t.eng.reinstall(ctx)
		return err
	}
	t.candidateRoot = nil
	return nil
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
// under the accepted World root, and the replay save the last write deferred
// with the span holding the operation's payloads, in one physical
// transaction after the publications of the writes admitted before it, and
// returns its receipt once the store admits it. The engine records the
// admission, and the write keeps the receipt until Discard. When head is nil
// or the store cannot publish, it writes the blocks and returns a nil receipt;
// the SharedObject then commits its state itself.
func (t *soEngineWriteTx) publish(ctx context.Context, publisher sobject.StatePublisher, root *block.BlockRef, head *block.AtomicHeadUpdate) (*block.PublicationReceipt, error) {
	// Fall back when the state or the store cannot join a publication.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/publish")
	defer task.End()
	store, ok := t.eng.so.GetBlockStore().(block.AtomicPublisher)
	if head == nil || !ok || !store.SupportsAtomicPublication() {
		return nil, t.writeBlocks(ctx)
	}

	// Borrow the candidate blocks and collect the pending replay save.
	batch, err := t.btx.TakePending(ctx)
	if err != nil {
		return nil, err
	}
	saveEntries, roots, cursor, err := t.eng.replay.pendingSave(publisher, t.fork.payloads)
	if err != nil {
		batch.Complete(err)
		return nil, err
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
		After:    t.eng.lastReceipt(),
		Ordered:  world.OrderedCommit(ctx),
	})
	if errors.Is(err, block.ErrAtomicPublicationUnsupported) || errors.Is(err, block.ErrPublicationTooLarge) {
		batch.Complete(err)
		return nil, t.writeBlocks(ctx)
	}
	if err != nil {
		batch.Complete(err)
		return nil, err
	}
	t.clock.end(stagePublicationAdmission)

	// The publication holds the candidate under the accepted World root and
	// carries the replay save once its receipt succeeds.
	held := t.fork.state.GetHeadRef().CloneVT()
	held.BucketId = t.eng.so.GetBlockStore().GetID()
	t.receipt, t.batch = receipt, batch
	t.eng.admit(receipt, held)
	t.fork.published = true
	return receipt, nil
}

// ApplyWorldOp applies op to the candidate as accepted replay will apply it.
// Replay attributes an authenticated operation to the verified signer, so the
// candidate runs it as this engine's device with the device's accepted person
// and role.
func (t *soEngineWriteTx) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	if _, authenticated := op.(world.AuthenticatedOperation); authenticated {
		participant, err := t.eng.operationParticipant(ctx)
		if err != nil {
			return 0, false, err
		}
		sender = t.eng.so.GetPeerID()
		ctx = withOperationAuthor(ctx, participant)
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
