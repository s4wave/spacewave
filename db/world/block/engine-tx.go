package world_block

import (
	"context"
	stderrors "errors"
	"os"
	"sync/atomic"
	"time"

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
	// recordedRoots are the roots operation references as payloads, set with
	// operation.
	// The commit keeps each beside the blocks its new root reaches, so a root
	// the batch set and then replaced or deleted, or a payload the World does
	// not reach, is still stored with the operation that references it.
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
	nextRootRef, adm, rootBlock, changes, releasePending, err := e.admit(ctx, turn)
	if releasePending != nil {
		defer releasePending()
	}
	if e.engine.authority == nil {
		return e.publishCallerHead(ctx, turn, nextRootRef, rootBlock, changes, err)
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

	// Wait outside the turn. On deadline, leave the queued operation with
	// the Authority: Release could remove it before the driver takes it.
	published, err := e.waitDurable(ctx, adm, nextRootRef)
	if !errors.Is(err, errDurabilityDeadline) {
		defer adm.Release()
	}
	if err != nil {
		return nil, err
	}
	if !published.EqualsRef(nextRootRef) {
		rootBlock = nil
		changes = nil
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
		if !e.engine.head.root.GetRef().EqualsRef(e.baseHeadRef) {
			changes = nil
		}
		var installErr error
		retirement, installErr = e.engine.installRootRefLocked(ctx, published, false, rootBlock, changes)
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
func (e *EngineTx) publishCallerHead(
	ctx context.Context,
	turn world.Turn,
	next *bucket.ObjectRef,
	rootBlock *World,
	changes *world.ChangeSet,
	err error,
) (*bucket.ObjectRef, error) {
	// Publish the head unless the caller replaced it.
	var retirement engineRetirement
	locked := e.engine.bcast.Lock()
	detached := e.engine.writeTx != e
	if err == nil && detached {
		err = e.endedErr()
	}
	if err == nil && !e.engine.closed && !next.EqualsRef(e.baseHeadRef) {
		if !e.engine.head.root.GetRef().EqualsRef(e.baseHeadRef) {
			changes = nil
		}
		retirement, err = e.engine.installRootRefLocked(ctx, next, false, rootBlock, changes)
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

// ProvisionalNoopGuardEnv set to "off" restores success for unchanged writes
// built on provisional heads. With the guard on, those writes must retry.
const ProvisionalNoopGuardEnv = "SPACEWAVE_WORLD_PROVISIONAL_NOOP_GUARD"

// admit seals the transaction's blocks under its new root and proposes them
// to the Authority as the head after the base. It returns the root, the
// Admission, the decoded World root for the head's readers, the transaction's
// ChangeSet, and a cleanup func for the pending ChangeSet registration when an
// Authority is present. A transaction that changed nothing needs no Admission.
func (e *EngineTx) admit(ctx context.Context, turn world.Turn) (*bucket.ObjectRef, world.Admission, *World, *world.ChangeSet, func(), error) {
	// Capture changes and seal the transaction's blocks under its new root.
	nroot, changes, err := e.writeTx.commitChanges(ctx)
	if isCoordinatedWriteSnapshotError(err) {
		err = errors.Wrap(world.ErrSuperseded, "commit world blocks")
	}
	if err == nil {
		err = nroot.Validate(false)
	}
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// An unchanged root is success only if the write did not depend on a
	// provisional predecessor. That predecessor can still be refused. The
	// unchanged candidate was never admitted, so the caller can retry it.
	base := e.baseHeadRef.GetRootRef()
	if nroot.EqualVT(base) {
		if e.engine.authority != nil {
			// A zero turn base is an empty World, not a pending candidate.
			turnBase := turn.Base()
			if !turnBase.Durable && !turnBase.Root.GetEmpty() && os.Getenv(ProvisionalNoopGuardEnv) != "off" {
				return nil, nil, nil, nil, nil, errors.Wrap(world.ErrSuperseded, "unchanged write on provisional World head")
			}
		}
		return e.baseHeadRef.Clone(), nil, nil, nil, nil, nil
	}
	next := e.baseHeadRef.Clone()
	next.RootRef = nroot

	// Borrow the blocks the new root and the recorded payload roots reach
	// from the write buffer, children before parents, and read the seqno and
	// root the head's readers need.
	batch, seqno, rootBlock, err := e.writeTx.takeCandidate(ctx, nroot, e.recordedRoots)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// Register the ChangeSet before Admit so followHead sees it if the
	// Authority publishes the candidate before Admit returns.
	var releasePending func()
	if e.engine.authority != nil {
		releasePending = e.engine.registerPendingChange(base, nroot, changes)
	}

	// Propose the candidate. Admit takes the entries: once it returns, the
	// buffer's copy is no longer needed.
	adm, err := turn.Admit(ctx, world.Candidate{
		Base:      base.Clone(),
		Root:      nroot.Clone(),
		Seqno:     seqno,
		Entries:   batch.Entries,
		Staged:    batch.Drained,
		Operation: e.operation,
	})
	batch.Complete(err)
	if err != nil {
		if releasePending != nil {
			releasePending()
		}
		return nil, nil, nil, nil, nil, err
	}
	return next, adm, rootBlock, changes, releasePending, nil
}

// DurabilityTimeoutEnv names the server-side budget for waiting on an
// admitted World write. It accepts a positive Go duration or "off" for the
// unbounded wait. An unset or invalid setting uses 30 seconds.
const DurabilityTimeoutEnv = "SPACEWAVE_WORLD_DURABILITY_TIMEOUT"

// defaultDurabilityTimeout bounds the wait when no valid override is set.
const defaultDurabilityTimeout = 30 * time.Second

// errDurabilityDeadline marks a wait that spent the server budget, not a
// decision by the Authority.
var errDurabilityDeadline = errors.Wrap(world.ErrDurabilityUnknown, "durability wait deadline exceeded")

// durabilityTimeout reads the current budget for a newly admitted write.
func durabilityTimeout() time.Duration {
	// Keep the previous unbounded wait available at runtime.
	value := os.Getenv(DurabilityTimeoutEnv)
	if value == "off" {
		return 0
	}

	// An invalid duration must not disable the bound.
	if budget, err := time.ParseDuration(value); err == nil && budget > 0 {
		return budget
	}
	return defaultDurabilityTimeout
}

// WithDurabilityTimeout bounds a wait for an already admitted World write.
// Expiry reports an unknown outcome, never a refusal or permission to replay.
func WithDurabilityTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if budget := durabilityTimeout(); budget > 0 {
		return context.WithTimeoutCause(ctx, budget, errDurabilityDeadline)
	}
	return context.WithCancel(ctx)
}

// waitDurable waits for the Authority's decision and record, and returns the
// head to publish. The writer turn has ended but the Authority still owns the
// admitted operation and its payloads. The request's cancellation cannot end
// the wait. Expiry leaves queued work with the Authority and never permits
// replay: neither elapsed time nor head progress proves that this candidate
// applied or was refused.
func (e *EngineTx) waitDurable(ctx context.Context, adm world.Admission, next *bucket.ObjectRef) (*bucket.ObjectRef, error) {
	// Keep the request values, but bound the wait by engine close or the
	// server's budget, with one budget for both waits and reconciliation.
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	defer cancel(nil)
	stop := context.AfterFunc(e.engine.closing, func() { cancel(world.ErrDurabilityUnknown) })
	defer stop()
	ctx, stopWait := WithDurabilityTimeout(ctx)
	defer stopWait()

	// A decision returned by the Authority can settle the write even if
	// the wait was canceled at the same time.
	decision, err := adm.WaitDecided(ctx)
	switch {
	case decision == world.Superseded:
		head := adm.Head()
		return nil, errors.Wrapf(world.ErrSuperseded, "head moved to seqno %d", head.Seqno)
	case decision == world.Undecided && ctx.Err() != nil:
		return nil, context.Cause(ctx)
	case decision != world.Accepted && err == nil:
		return nil, errors.Errorf("world candidate %s", decision)
	case err != nil:
		return nil, err
	}

	// Wait for the record that makes the head durable, and publish the head
	// the Admission reports, which holds the candidate's change.
	if _, err = adm.WaitDurable(ctx); err == nil {
		if head := adm.Head(); !head.Root.GetEmpty() && !head.Root.EqualsRef(next.GetRootRef()) {
			published := next.Clone()
			published.RootRef = head.Root.Clone()
			return published, nil
		}
		return next, nil
	}

	// Reconcile a lost report only while the wait is live. An Authority's
	// Head may return a result even when its context ended during the read.
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	head, herr := e.engine.authority.Head(ctx)
	switch {
	case ctx.Err() != nil:
		return nil, context.Cause(ctx)
	case herr != nil:
		// The Authority accepted the candidate, so a failed reconcile leaves
		// its outcome unknown. The head read's error stays text: its
		// sentinels, such as a stale generation, would read as a refusal and
		// replay a write that may be durable.
		if !errors.Is(err, world.ErrDurabilityUnknown) {
			err = stderrors.Join(world.ErrDurabilityUnknown, err)
		}
		return nil, errors.Wrapf(err, "read head: %v", herr)
	// For a live wait, the accepted candidate is on the Authority's linear
	// order: its root or a later head confirms an ordinary lost report.
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
