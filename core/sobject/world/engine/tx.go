package sobject_world_engine

import (
	"context"
	"errors"
	fmt "fmt"
	"slices"
	"sync"

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
	// prepared retains the candidate borrow across an unlocked capacity wait.
	prepared *block.PendingBatch
	// preparedSave is the replay save sized with prepared under the writer lock.
	preparedSave   []*block.PutBatchEntry
	preparedRoots  []block.NamedRoot
	preparedCursor *block.AtomicHeadUpdate
	// reservation owns admission capacity before the provider state writer.
	reservation block.PublicationReservation
	// baseReceipt is the predecessor of the candidate before its unlocked wait.
	baseReceipt *block.PublicationReceipt
	// publicationEpoch is the original attempt's publication history.
	publicationEpoch uint64
	// admissionPreparation carries pre-lock capacity into a rerun body.
	admissionPreparation *admissionPreparation
	// stateReservation excludes other state writers during commit preparation.
	stateReservation sobject.StateWriteReservation
	// stateSettled permits a declined publication to fall back to a direct write.
	stateSettled bool
	// releasePreparation lets the next candidate start once this one is admitted.
	releasePreparation func()
	// clock times the stages of the write.
	clock *commitClock
	// seqno is the World seqno after the write, set at admission.
	seqno uint64
	// durable is the durable head's install of the World after the write,
	// which readers see once the durable head installs it, or nil when the
	// engine has a single head.
	durable *durableInstall
	// before is the seqno of the World before the write's publication.
	before uint64
	// admission is set once CommitAdmit returns it. Discard then leaves the
	// write to the admission's Release.
	admission *soAdmission
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
		WorldState:       worldState,
		btx:              btx,
		eng:              eng,
		fork:             fork,
		unlockWriteMtx:   unlockWriteMtx,
		clock:            clock,
		baseReceipt:      eng.lastReceipt(),
		publicationEpoch: eng.publicationEpoch.Load(),
	}
}

// Commit commits the write at admission, then waits until it is durable and
// readers see it. A write is acknowledged only after its receipt.
func (t *soEngineWriteTx) Commit(ctx context.Context) error {
	// Admit the write and wait for its durable result.
	adm, err := t.CommitAdmit(ctx)
	if err != nil {
		return err
	}
	defer adm.Release()
	_, err = adm.WaitDurable(ctx)
	return err
}

// CommitAdmit persists the candidate blocks, adds the transaction to the
// operation set and installs the World after it for writers. When no other
// operation arrived since the fork, the candidate is that World; otherwise
// replay computes it. A SharedObject that publishes its state commits the
// blocks, the operation and the accepted World in one physical transaction:
// CommitAdmit releases the writer lock and returns once the publication
// admits the write and the engine installs its World, so the next write forks
// from it. The Admission waits for the receipt. Without a publication the
// write is durable when CommitAdmit returns. A write whose World follows a
// publication that failed before its admission fails with that failure; it
// is never replayed onto the reinstalled World, and CommitAdmit runs no
// reinstall hook.
func (t *soEngineWriteTx) CommitAdmit(ctx context.Context) (world.Admission, error) {
	// Admit the write.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/commit")
	defer task.End()
	err := t.admit(ctx)

	// A pending publication waits in the Admission.
	if err == nil && t.receipt != nil {
		t.admission = &soAdmission{t: t}
		return t.admission, nil
	}

	// Otherwise finish here. A failed placement still reports the receipt's
	// failure first, since queueOperation may report only what followed it.
	defer t.Discard()
	defer t.clock.finish(t.eng.c.le)
	if t.receipt != nil {
		if rerr := t.waitReceipt(ctx); rerr != nil {
			return nil, fmt.Errorf("world publication receipt: %w", rerr)
		}
	}
	if err == nil {
		waitCtx, cancel := t.eng.detach(ctx)
		_, _, err = t.eng.waitDurable(waitCtx, t.durable)
		cancel()
	}
	if err != nil {
		return nil, err
	}
	return world.DurableAdmission(t.seqno), nil
}

// admit adds the operation and installs the World after it, then releases
// the writer lock. It records the World seqno and its durable position.
func (t *soEngineWriteTx) admit(ctx context.Context) error {
	// End the callback stage.
	t.clock.end(stageCallback)
	if err := t.checkPublicationBase(); err != nil {
		return err
	}

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
		var err error
		t.seqno, err = t.eng.admitted.GetSeqno(ctx)
		return err
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

	// Claim capacity outside writer locks before the operation takes the state lock.
	if publishes {
		var err error
		settled := t.admissionPreparation != nil && t.admissionPreparation.retry.settled
		ctx, err = t.reserveStateWrite(ctx, settled)
		if err != nil {
			return err
		}
		defer t.releaseStateWrite()
		if err := t.reservePublication(ctx, publisher); err != nil {
			return err
		}
	}

	// Add the operation and install the World after it.
	taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/queue-operation")
	defer task.End()
	if publishes {
		if t.reservation != nil {
			defer t.reservation.Release()
		}
		taskCtx = sobject.WithPublishState(taskCtx, func(ctx context.Context, head *block.AtomicHeadUpdate) (*block.PublicationReceipt, error) {
			return t.publish(ctx, publisher, nroot, head)
		})
	}
	for {
		err = t.eng.queueOperation(taskCtx, opData, t.fork, t.clock)
		if !errors.Is(err, sobject.ErrStateWriteUnsettled) || t.stateSettled {
			break
		}

		// A caller holding its own writer discards before preparing a fresh body.
		if world.CanRetryAdmission(taskCtx) {
			return t.admissionRetry(0, true, true)
		}

		// A declined atomic write must drain outside both writers, then retry
		// against the durable state without changing its original predecessor.
		t.unlockWriteMtx()
		t.releaseStateWrite()
		if t.reservation != nil {
			t.reservation.Release()
			t.reservation = nil
		}
		taskCtx, err = t.reserveStateWrite(taskCtx, true)
		if err != nil {
			return err
		}
		if err = t.reservePublication(taskCtx, publisher); err != nil {
			return err
		}
	}
	t.clock.end(stageAdvance)

	// Record the World after the write and release the writer lock.
	if err == nil {
		t.seqno, err = t.eng.admitted.GetSeqno(ctx)
		t.durable = t.eng.lastDurable()
	}
	t.unlockWriteMtx()
	if t.releasePreparation != nil {
		t.releasePreparation()
	}
	if err != nil {
		return fmt.Errorf("queue World operation: %w", err)
	}
	return nil
}

// waitReceipt waits for the publication to resolve, finishes the borrow of the
// candidate blocks with its result and returns its error. A failed
// publication returns at once: the receipt's watcher reinstalls the World
// from before it, and the next write settles the failure before it forks, so
// the writer never waits for the reinstall or its hooks. Admission commits
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

	// The accepted World root holds a published candidate.
	err = receipt.Err()
	batch.Complete(err)
	if err != nil {
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
// returns its receipt once the store admits it. With the page view the World
// pages stay kept instead, and the publication names no World: a checkpoint
// or a stall base writes the Worlds a restart reads. The engine records the
// admission, and the write keeps the receipt until Discard. When head is nil
// or the store cannot publish, it writes the blocks and returns a nil receipt;
// the SharedObject then commits its state itself.
func (t *soEngineWriteTx) publish(ctx context.Context, publisher sobject.StatePublisher, root *block.BlockRef, head *block.AtomicHeadUpdate) (*block.PublicationReceipt, error) {
	// A candidate prepared on a failed admission never enters a later chain.
	if err := t.checkPublicationBase(); err != nil {
		return nil, err
	}

	// Fall back when the state or the store cannot join a publication.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/publish")
	defer task.End()
	store, ok := t.eng.so.GetBlockStore().(block.AtomicPublisher)
	if head == nil || !ok || !store.SupportsAtomicPublication() {
		if t.stateReservation != nil && !t.stateSettled {
			return nil, sobject.ErrStateWriteUnsettled
		}
		if t.prepared != nil {
			t.prepared.Complete(block.ErrAtomicPublicationUnsupported)
			t.prepared = nil
		}
		return nil, t.writeBlocks(ctx)
	}

	// Read the seqno of the World before the write, which a failed receipt
	// reinstalls.
	before, err := t.eng.admitted.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}

	// Borrow the candidate blocks and collect the pending replay save.
	batch := t.prepared
	prepared := batch != nil
	t.prepared = nil
	if batch == nil {
		batch, err = t.btx.TakePending(ctx)
		if err != nil {
			return nil, err
		}
	}
	saveEntries, roots, cursor := t.preparedSave, t.preparedRoots, t.preparedCursor
	if !prepared {
		saveEntries, roots, cursor, err = t.eng.replay.pendingSave(ctx, publisher, t.fork.payloads)
		if err != nil {
			batch.Complete(err)
			return nil, err
		}
	}

	// Keep the World pages, which a reader of the new head finds in the page
	// view, and publish the payloads with the pages past its bound.
	payload, pages := splitPayloads(batch.Entries, t.fork.payloads)
	pub := &block.AtomicPublication{
		RootName: acceptedWorldRootName,
		Root:     root,
		Roots:    roots,
		After:    t.eng.lastReceipt(),
		Ordered:  world.OrderedCommit(ctx),
		Recount:  t.eng.replay.ledger.carryRecount(),
	}
	if kept := t.eng.replay.pages; kept != nil {
		pages = kept.keep(pages)
		pub.RootName, pub.Root = "", nil
	}
	pub.Entries = slices.Concat(payload, pages, saveEntries)
	named, save := t.eng.replay.pages.split(saveEntries)
	pub.Heads = []*block.AtomicHeadUpdate{countHead(head, WriteKindState)}
	if cursor != nil {
		pub.Heads = append(pub.Heads, countHead(cursor, WriteKindReplay))
	}

	// Submit them, writing the blocks alone when the store declines.
	pubCtx := ctx
	if t.reservation != nil {
		pubCtx = block.WithPublicationReservation(ctx, t.reservation)
	}
	receipt, err := store.SubmitAtomic(pubCtx, pub)
	if errors.Is(err, block.ErrAtomicPublicationUnsupported) || errors.Is(err, block.ErrPublicationTooLarge) {
		if t.stateReservation != nil && !t.stateSettled {
			t.prepared = batch
			return nil, sobject.ErrStateWriteUnsettled
		}
		batch.Complete(err)
		return nil, t.writeBlocks(ctx)
	}
	if err != nil {
		batch.Complete(err)
		return nil, err
	}
	t.clock.end(stagePublicationAdmission)
	countPublication(payload, slices.Concat(pages, named), save)
	t.eng.replay.addWorldPut(receipt, pub.Entries, pages)

	// The publication holds the candidate under the accepted World root and
	// carries the replay save once its receipt succeeds.
	held := t.fork.state.GetHeadRef().CloneVT()
	held.BucketId = t.eng.so.GetBlockStore().GetID()
	t.receipt, t.batch, t.before = receipt, batch, before
	t.eng.admit(receipt, held, before)
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
	// An admitted write ends with its Admission.
	if t.admission != nil {
		return
	}
	t.discard()
}

// discard drops the candidate and releases the write.
func (t *soEngineWriteTx) discard() {
	// Drop the candidate and let the next writer start.
	t.releaseStateWrite()
	if t.prepared != nil {
		t.prepared.Complete(context.Canceled)
		t.prepared = nil
	}
	if t.reservation != nil {
		t.reservation.Release()
	}
	if t.releasePreparation != nil {
		t.releasePreparation()
	}
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

// soAdmission is the Admission of a write whose publication was admitted.
type soAdmission struct {
	t *soEngineWriteTx
	// wait waits once for the durable result.
	wait     sync.Once
	waitedAt uint64
	waitErr  error
	// release releases the write once.
	release sync.Once
}

// Seqno returns the World seqno after the write.
func (a *soAdmission) Seqno() uint64 {
	return a.t.seqno
}

// WaitDurable waits for the receipt, then for the durable head to install or
// drop the write, and returns the seqno readers see. A failed write returns
// once the durable head dropped it, with the seqno of the World readers kept,
// which is the World the engine reinstalls: neither waits for the writer
// lock. Later calls return the first result.
func (a *soAdmission) WaitDurable(ctx context.Context) (uint64, error) {
	a.wait.Do(func() {
		// Wait for the receipt.
		t := a.t
		rerr := t.waitReceipt(ctx)
		if rerr != nil && t.receipt == nil {
			// The wait ended with the engine: the outcome is unknown.
			a.waitErr = fmt.Errorf("world publication receipt: %w", rerr)
			return
		}
		waitCtx, cancel := t.eng.detach(ctx)
		defer cancel()
		dropped, kept, err := t.eng.waitDurable(waitCtx, t.durable)
		switch {
		case rerr != nil && dropped:
			a.waitedAt, a.waitErr = kept, fmt.Errorf("world publication receipt: %w", rerr)
		case rerr != nil:
			a.waitedAt, a.waitErr = t.before, fmt.Errorf("world publication receipt: %w", rerr)
		default:
			a.waitedAt, a.waitErr = t.seqno, err
		}
	})
	return a.waitedAt, a.waitErr
}

// Release releases the write and its receipt. The receipt is released once it
// resolves.
func (a *soAdmission) Release() {
	a.release.Do(func() {
		a.t.discard()
		a.t.clock.finish(a.t.eng.c.le)
	})
}

// _ is a type assertion
var (
	_ world.Tx             = (*soEngineWriteTx)(nil)
	_ world.AdmitCommitter = (*soEngineWriteTx)(nil)
	_ world.Admission      = (*soAdmission)(nil)
)
