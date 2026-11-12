package world_block

import (
	"context"
	stderrors "errors"
	"slices"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// EngineTx is an engine transaction wrapping the Tx object, returned by
// Engine.NewTransaction.
type EngineTx struct {
	// rel is set once the transaction committed or was discarded.
	rel atomic.Bool
	// engine is the engine the transaction belongs to.
	engine *Engine

	// readTx is read without a lock. A read snapshot replaces it under
	// Engine.snapMu.
	readTx atomic.Pointer[Tx]
	// readRoot is the World root a read snapshot opened at, which a storage
	// refresh reopens and the read-root roster retains.
	readRoot *bucket.ObjectRef
	// readHead holds the head a read snapshot opened at, so its object
	// reads follow refs without the Engine lock. Swapped out under
	// Engine.snapMu when the snapshot detaches or reopens its storage.
	readHead atomic.Pointer[heldHead]
	// writeTx is the mutable transaction, nil for a read snapshot.
	writeTx *Tx
	// baseHeadRef is the head the write transaction builds on.
	baseHeadRef *bucket.ObjectRef
	// privateBase is the cursor at a base the engine has not published, such
	// as an admitted candidate's root, owned by the transaction until it
	// retires. Nil when the transaction builds on the published head.
	privateBase *bucket_lookup.Cursor
	// operation is the encoded World operation batch a recording
	// transaction proposes as its Candidate's Operation, set before Commit.
	operation []byte
	// recordedRoots are the object roots operation records, set with it. The
	// commit keeps each beside the blocks its new root reaches, so a root the
	// batch set and then replaced or deleted is still stored with the
	// operation that names it.
	recordedRoots []*block.BlockRef
	// turn is the Authority's write turn, held from the open until the commit
	// is decided or the transaction is discarded. Moved out under the Engine
	// lock.
	turn world.Turn
	// committing is set under the Engine lock once Commit owns the
	// transaction, so Close leaves it, and its stage, to the commit.
	committing bool
}

// newEngineTx constructs a new EngineTx.
func newEngineTx(e *Engine, writeTx *Tx) *EngineTx {
	return &EngineTx{writeTx: writeTx, engine: e}
}

// Fork forks the current world state into a completely separate world state.
//
// Creates a new block transaction.
func (e *EngineTx) Fork(ctx context.Context) (world.WorldState, error) {
	return performOpValue(ctx, e, func(tx *Tx) (world.WorldState, error) {
		return tx.Fork(ctx)
	})
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// If not write, returns ErrNotWrite.
func (e *EngineTx) Commit(ctx context.Context) error {
	_, err := e.CommitBlockTransaction(ctx)
	return err
}

// CommitBlockTransaction implements Commit but additionally returns the updated ObjectRef.
// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// If not write, returns ErrNotWrite.
//
// The commit runs the World's commit state machine over the Authority. Open
// is the transaction itself. Sealing its blocks and proposing them as a
// Candidate moves it to Admitted, which ends the turn and the writer slot, so
// the next writer begins on the candidate while this one waits. The
// Authority's decision moves it to Superseded, which returns
// world.ErrSuperseded so the writer rebases, or on to Durable, when the
// engine publishes the head the Admission reports, which a Space's Authority
// may have rebased onto operations the writer did not build on. The local
// Authority decides and makes the head durable within Admit, in one Volume
// transaction.
func (e *EngineTx) CommitBlockTransaction(ctx context.Context) (*bucket.ObjectRef, error) {
	// Start the commit trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction")
	defer task.End()

	// Require a writable transaction.
	if e.writeTx == nil {
		e.Discard()
		return nil, tx.ErrNotWrite
	}

	// Set rel before moving the turn out of Engine state. A commit that finds
	// rel already set returns ErrDiscarded.
	if e.rel.Swap(true) {
		return nil, e.endedErr()
	}

	// Move the turn into commit-local state.
	locked := e.engine.bcast.Lock()
	if e.engine.writeTx != e {
		locked.Unlock()
		return nil, e.endedErr()
	}
	turn := e.turn
	e.turn = nil
	e.committing = true
	e.engine.committing++
	defer e.engine.finishCommit()
	locked.Unlock()

	// Seal the transaction's blocks under its new root and propose them.
	nextRootRef, adm, rootBlock, err := e.admit(ctx, turn)
	if adm != nil {
		defer adm.Release()
	}
	if e.engine.authority == nil {
		return e.publishCallerHead(ctx, turn, nextRootRef, rootBlock, err)
	}

	// Record the published head at admission, and the candidate the next
	// writer may build on.
	locked = e.engine.bcast.Lock()
	admittedAt := e.engine.pub.Load()
	if err == nil && adm != nil {
		e.engine.admitted = &admittedHead{root: nextRootRef.GetRootRef(), world: rootBlock}
	}

	// End the turn and the writer slot now that the Authority holds the
	// candidate.
	var writer engineRetirement
	if e.engine.writeTx == e {
		writer = e.detachLocked()
	}
	writer.turn = turn
	retirement := e.engine.beginRetirementLocked(writer)
	locked.Unlock()
	e.engine.drainRetirement(ctx, retirement)
	if err != nil || adm == nil {
		return nextRootRef, err
	}

	// Wait for the decision and for the head to be durable, outside the turn.
	published, err := e.waitDurable(ctx, adm, nextRootRef)
	if err != nil {
		return nil, err
	}
	if !published.EqualsRef(nextRootRef) {
		rootBlock = nil
	}

	// Publish the head, unless a head was published since the admission: the
	// next writer's durable base or a head the watch reported, which holds
	// this candidate's change or a later one. A durable commit stays a
	// success when the engine closed meanwhile, or the install failed: the
	// Authority holds its head, and the head watch or the next open installs
	// it.
	locked = e.engine.bcast.Lock()
	retirement = engineRetirement{}
	if !e.engine.closed && e.engine.pub.Load() == admittedAt {
		var installErr error
		retirement, installErr = e.engine.installRootRefLocked(ctx, published, false, rootBlock)
		if installErr != nil {
			e.engine.le.WithError(installErr).Warn("unable to publish a durable World head; the authority holds it")
		} else if p := e.engine.pub.Load(); p != nil {
			e.engine.direct = &world.Head{Root: published.GetRootRef().Clone(), Seqno: p.seqno, Durable: true}
		}
	}
	locked.Unlock()
	e.engine.drainRetirement(ctx, retirement)
	return published, nil
}

// publishCallerHead finishes the commit of an engine without an Authority,
// whose caller owns the head and may replace it while the commit runs: the
// writer keeps its slot until its head is published, and a commit whose
// writer the caller detached is discarded.
func (e *EngineTx) publishCallerHead(ctx context.Context, turn world.Turn, next *bucket.ObjectRef, rootBlock *World, err error) (*bucket.ObjectRef, error) {
	// Publish the head unless the caller replaced it.
	var retirement engineRetirement
	locked := e.engine.bcast.Lock()
	detached := e.engine.writeTx != e
	if err == nil && detached {
		err = e.endedErr()
	}
	if err == nil && !e.engine.closed && !next.EqualsRef(e.baseHeadRef) {
		retirement, err = e.engine.installRootRefLocked(ctx, next, false, rootBlock)
	}

	// Detach this writer even when the commit failed, and end its turn
	// before its writer slot in the same retirement.
	var writer engineRetirement
	if e.engine.writeTx == e {
		writer = e.detachLocked()
	}
	writer.turn = turn
	if retirement.empty() {
		retirement = e.engine.beginRetirementLocked(writer)
	} else {
		retirement.readTx, retirement.readHead, retirement.writeTx = writer.readTx, writer.readHead, writer.writeTx
		retirement.turn, retirement.base, retirement.writeTxRel = writer.turn, writer.base, writer.writeTxRel
	}
	locked.Unlock()
	e.engine.drainRetirement(ctx, retirement)
	if err != nil {
		return nil, err
	}
	return next, nil
}

// admit seals the transaction's blocks under its new root and proposes them
// to the Authority as the head after the base. It returns the root, the
// Admission, and the decoded World root for the head's readers. A
// transaction that changed nothing returns its base and no Admission.
func (e *EngineTx) admit(ctx context.Context, turn world.Turn) (*bucket.ObjectRef, world.Admission, *World, error) {
	// Seal the transaction's blocks under its new root.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/write-tx-commit")
	nroot, err := e.writeTx.CommitBlockTransaction(taskCtx)
	subtask.End()
	if isCoordinatedWriteSnapshotError(err) {
		err = errors.Wrap(world.ErrSuperseded, "commit world blocks")
	}
	if err == nil {
		err = nroot.Validate(false)
	}
	if err != nil {
		return nil, nil, nil, err
	}

	// A transaction that changed no root data has nothing to propose.
	base := e.baseHeadRef.GetRootRef()
	if nroot.EqualVT(base) {
		return e.baseHeadRef.Clone(), nil, nil, nil
	}
	next := e.baseHeadRef.Clone()
	next.RootRef = nroot

	// Borrow the blocks the new root and the recorded object roots reach
	// from the write buffer, children before parents, and read the seqno and
	// root the head's readers need.
	state := e.writeTx.state
	batch, err := state.ownedStore.TakeReachable(ctx, slices.Concat([]*block.BlockRef{nroot}, e.recordedRoots)...)
	if err != nil {
		return nil, nil, nil, err
	}
	seqno, err := state.GetSeqno(ctx)
	var rootBlock *World
	if err == nil {
		rootBlock, err = state.GetRoot(ctx)
	}
	if err != nil {
		batch.Complete(err)
		return nil, nil, nil, err
	}

	// Propose the candidate. Admit takes the entries: once it returns, the
	// buffer's copy is no longer needed.
	adm, err := turn.Admit(ctx, world.Candidate{
		Base:      base.Clone(),
		Root:      nroot.Clone(),
		Seqno:     seqno,
		Entries:   batch.Entries,
		Operation: e.operation,
	})
	batch.Complete(err)
	if err != nil {
		return nil, nil, nil, err
	}
	return next, adm, rootBlock, nil
}

// waitDurable waits for the Authority's decision and then for the head to be
// durable, and returns the head to publish. Admission committed the writer to
// the outcome, so the waits outlive the request context, until the engine
// closes, which ends them with world.ErrDurabilityUnknown. A superseded
// candidate returns world.ErrSuperseded naming the head that won.
//
// A wait for durability that fails after the Authority accepted the candidate
// leaves its durability unknown, a state rather than a failure: the engine
// reconciles it with the Authority's head. A head at the candidate's root, or
// past the candidate's seqno, which the Authority's linear order can only
// reach through the candidate, means the commit is durable, and that head is
// published. Any other head means the candidate is gone: Superseded. A commit
// the Authority made durable is never reported failed.
func (e *EngineTx) waitDurable(ctx context.Context, adm world.Admission, next *bucket.ObjectRef) (*bucket.ObjectRef, error) {
	// Wait for the decision past the request's cancellation, until the
	// engine closes, which leaves the durability unknown.
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	defer cancel(nil)
	stop := context.AfterFunc(e.engine.closing, func() { cancel(world.ErrDurabilityUnknown) })
	defer stop()
	decision, err := adm.WaitDecided(ctx)
	switch {
	case decision == world.Superseded:
		head := adm.Head()
		return nil, errors.Wrapf(world.ErrSuperseded, "head moved to seqno %d", head.Seqno)
	case decision != world.Accepted && err == nil:
		return nil, errors.Errorf("world candidate %s", decision)
	case err != nil:
		return nil, err
	}

	// Wait for the record that makes the head durable, and publish the head
	// the Admission reports, which holds the candidate's change.
	if _, err = adm.WaitDurable(ctx); err == nil || e.engine.authority == nil {
		if head := adm.Head(); err == nil && !head.Root.GetEmpty() && !head.Root.EqualsRef(next.GetRootRef()) {
			published := next.Clone()
			published.RootRef = head.Root.Clone()
			return published, nil
		}
		return next, err
	}

	// Reconcile an unknown durability with the Authority's head.
	head, herr := e.engine.authority.Head(ctx)
	switch {
	case herr != nil:
		return nil, stderrors.Join(err, herr)
	case head.Root.EqualsRef(next.GetRootRef()):
		return next, nil
	case head.Seqno > adm.Seqno():
		published := next.Clone()
		published.RootRef = head.Root.Clone()
		return published, nil
	case errors.Is(err, world.ErrDurabilityUnknown):
		return nil, err
	}
	return nil, stderrors.Join(errors.Wrapf(world.ErrSuperseded, "candidate lost: head at seqno %d", head.Seqno), err)
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (e *EngineTx) Discard() {
	// Stop when another lifecycle path already released this transaction.
	if e.rel.Swap(true) {
		return
	}

	// A read snapshot detaches without the Engine lock, so releasing it never
	// waits on a writer holding bcast.
	if e.writeTx == nil {
		e.engine.discardSnapshot(e)
		return
	}

	// Detach under the Engine lock, then drain after unlocking.
	locked := e.engine.bcast.Lock()
	retirement := e.engine.beginRetirementLocked(e.detachLocked())
	locked.Unlock()
	e.engine.drainRetirement(context.Background(), retirement)
}

// endedErr returns the error an operation on a transaction the engine or its
// owner ended reports: errEngineRetired once the engine retired, and
// tx.ErrDiscarded otherwise.
func (e *EngineTx) endedErr() error {
	if e.engine.retired.Load() {
		return errEngineRetired
	}
	return tx.ErrDiscarded
}

// detachSnapshotLocked removes this read snapshot from Engine.snapshotTxs
// and returns its state and held head for release. It returns nothing when
// another path already detached it. The caller must hold Engine.snapMu.
func (e *EngineTx) detachSnapshotLocked() engineRetirement {
	// Mark the snapshot discarded, then detach it exactly once.
	e.rel.Store(true)
	if _, ok := e.engine.snapshotTxs[e]; !ok {
		return engineRetirement{}
	}
	delete(e.engine.snapshotTxs, e)
	return engineRetirement{readTx: e.readTx.Load(), readHead: e.readHead.Swap(nil)}
}

// detachLocked removes this write transaction from Engine.writeTx without
// waiting for its transaction locks or Authority turn.
func (e *EngineTx) detachLocked() engineRetirement {
	// Mark the transaction discarded before removing its Engine registrations.
	e.rel.Store(true)
	retirement := engineRetirement{
		readTx:   e.readTx.Load(),
		readHead: e.readHead.Swap(nil),
		writeTx:  e.writeTx,
		turn:     e.turn,
		base:     e.privateBase,
	}
	e.turn = nil
	e.privateBase = nil

	// Move Engine.writeTxRel into the retirement, which releases the serialized
	// write turn after the Engine lock drops.
	if e.engine.writeTx == e {
		e.engine.writeTx = nil
		retirement.writeTxRel = e.engine.writeTxRel
		e.engine.writeTxRel = nil
	}
	return retirement
}

// GetReadOnly returns if the state is read-only.
func (e *EngineTx) GetReadOnly() bool {
	return e.writeTx == nil
}

// Sync fences durable storage through the engine.
func (e *EngineTx) Sync(ctx context.Context) (bool, error) {
	return e.engine.Sync(ctx)
}

// _ is a type assertion
var (
	_ world.Tx                 = (*EngineTx)(nil)
	_ world.WorldState         = (*EngineTx)(nil)
	_ world.ForkableWorldState = (*EngineTx)(nil)
)
