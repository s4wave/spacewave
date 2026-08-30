package world_block

import (
	"context"

	trace "github.com/s4wave/spacewave/db/traceutil"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

type engineHead struct {
	root   *bucket_lookup.Cursor
	readTx *Tx
}

// engineRetirement is an obsolete resource set detached from Engine publication.
// The Engine drains it after unlocking so cleanup can reenter Engine state.
type engineRetirement struct {
	head       *engineHead
	readTx     *Tx
	writeTx    *Tx
	lease      coord.WriteLease
	writeTxRel func()
}

func (r engineRetirement) empty() bool {
	return r.head == nil &&
		r.readTx == nil &&
		r.writeTx == nil &&
		r.lease == nil &&
		r.writeTxRel == nil
}

// Engine is the world engine instance.
// Uses short-lived block graph transactions internally.
// Reads are against latest state; read txs don't lock.
// Re-tries transaction operations if the underlying transaction is discarded mid-way through.
// Maintains two WorldState objects: one for readers, one for writer.
type Engine struct {
	// le is the logger
	le *logrus.Entry
	// lookupOp looks up a world operation.
	lookupOp world.LookupOp
	// verbose enables verbose logging within world state
	verbose bool
	// wmtx ensures only one write transaction is active at a time
	wmtx csync.Mutex
	// bcast guards Engine head, transaction, lifecycle, and close state.
	bcast broadcast.Broadcast
	// baseRoot is the base root cursor to use.
	// the root cursor is derived with FollowRef from this cursor.
	baseRoot *bucket_lookup.Cursor
	// head pairs the root cursor with its shared read transaction.
	head *engineHead
	// writeTx is the current write tx
	// canceled if the state changes mid-write
	writeTx *EngineTx
	// writeTxRel releases wmtx, call when unsetting writeTx
	writeTxRel func()
	// coordinatorTxs tracks dedicated read snapshots owned by caller-held EngineTx values.
	coordinatorTxs map[*EngineTx]struct{}
	// retiring counts detached resource sets that have not finished draining.
	retiring int
	// committing counts write commits that own coordinator cleanup outside bcast.
	committing int
	// commitFn is a function to be called just before a commit is confirmed.
	// can be nil
	commitFn CommitFn
	// durableHeadRef is the head last written to durable storage via commitFn.
	// Distinct from root, which tracks the current in-memory root. In the
	// single-writer path the durable head is advanced only by Sync, so root can
	// run ahead of durableHeadRef between fences. Guarded by bcast.
	durableHeadRef *bucket.ObjectRef
	// writeBlockStore is the long-lived block store backing write transactions.
	// In the single-writer path with a store that is not self-buffered it is a
	// deferred BufferedStore wrapping the bucket store: block writes accumulate
	// and become durable only at Sync. Otherwise it is the bucket store directly
	// (coordinator mode stays durable-on-write; self-buffered stores own their
	// own intake + Sync fence). Stable for the engine lifetime because the world
	// cursor never switches buckets.
	writeBlockStore block.StoreOps
	// deferDurability enables the single-writer deferred-durability mode: block
	// writes and the durable head advance batch until Sync instead of becoming
	// durable per commit. Opt-in because callers that propose blocks to a remote
	// authority or write-then-exit need durable-on-write semantics. Ignored when
	// a write coordinator is configured (coordinator mode stays durable-on-write).
	deferDurability bool
	// writeCoordinator gates standalone ObjectStore-backed writers.
	writeCoordinator coord.Coordinator
	// writeCoordScope identifies this engine's durable ObjectStore head.
	writeCoordScope coord.Scope
	// writeCoordKeyPrefix is invalidated when the durable World head changes.
	writeCoordKeyPrefix []byte
	// writeHeadRefresh rereads the durable World head before a write mutation starts.
	writeHeadRefresh func(context.Context) (*bucket.ObjectRef, error)
	// closed is set after Close releases cursor and transaction resources.
	closed bool
}

// CommitFn is a function to call with the updated root before confirming it.
// Should be used to write the updated state back to storage.
// Note: the Engine state guard is held while cb runs; do not block or call Engine methods.
// If an error is returned the change will be rolled back.
// Do not change the nrootBcs during this call.
type CommitFn func(ctx context.Context, baseRef, nref *bucket.ObjectRef) error

// EngineOption configures optional Engine integrations.
type EngineOption func(*Engine)

// WithDeferredDurability enables single-writer deferred durability: block writes
// and the durable head advance batch in memory until Sync. It is the IC-3
// hot-path fence and has no effect when a write coordinator is configured.
func WithDeferredDurability() EngineOption {
	return func(e *Engine) {
		e.deferDurability = true
	}
}

// WithWriteCoordinator requires write transactions to acquire a coordinator
// lease and refresh the durable World head before mutation.
func WithWriteCoordinator(
	coordinator coord.Coordinator,
	scope coord.Scope,
	keyPrefix []byte,
	refreshHead func(context.Context) (*bucket.ObjectRef, error),
) EngineOption {
	return func(e *Engine) {
		e.writeCoordinator = coordinator
		e.writeCoordScope = scope
		e.writeCoordKeyPrefix = append([]byte(nil), keyPrefix...)
		e.writeHeadRefresh = refreshHead
	}
}

// NewEngine constructs a new world engine.
// commitFn can be nil.
func NewEngine(
	ctx context.Context,
	le *logrus.Entry,
	root *bucket_lookup.Cursor,
	lookupOp world.LookupOp,
	commitFn CommitFn,
	verbose bool,
	opts ...EngineOption,
) (*Engine, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new")
	defer task.End()

	e := &Engine{
		le:             le,
		baseRoot:       root,
		lookupOp:       lookupOp,
		head:           &engineHead{root: root.Clone()},
		commitFn:       commitFn,
		durableHeadRef: root.GetRef().Clone(),
		verbose:        verbose,
		coordinatorTxs: make(map[*EngineTx]struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}

	// In the opt-in single-writer deferred-durability mode, defer block durability
	// so per-commit writes accumulate and become durable only at Sync, alongside
	// the deferred durable head. Coordinator mode and all callers that did not opt
	// in stay durable-on-write and use the bucket directly.
	rawWriteStore := e.head.root.GetBucket()
	e.writeBlockStore = rawWriteStore
	if e.deferDurability && e.writeCoordinator == nil {
		if rawWriteStore.GetSupportedFeatures()&block.StoreFeatureSelfBuffered == 0 {
			// Not self-buffered (e.g. bbolt): defer behind one long-lived
			// BufferedStore that accumulates writes in memory until Sync.
			e.writeBlockStore = block.NewBufferedStore(ctx, rawWriteStore)
		}
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new/initialize-head-read-tx")
	err := e.initializeHeadReadTx(taskCtx)
	subtask.End()
	if err != nil {
		_ = e.Close()
		return nil, err
	}
	return e, nil
}

// GetRootRef gets the current root cursor reference.
func (e *Engine) GetRootRef() *bucket.ObjectRef {
	locked := e.bcast.Lock()
	ref := e.head.root.GetRef().Clone()
	locked.Unlock()
	return ref
}

// Sync fences durable storage and advances the durable world head.
//
// The fence is ordered: first the block barrier makes every block written so
// far durable, then the durable head is advanced to the current in-memory root
// via commitFn. Ordering matters because a head that names not-yet-durable
// blocks is the crash window this fence closes.
//
// In the single-writer path (no write coordinator) the per-commit path defers
// the durable head, so this is where it advances. In coordinator mode the head
// is already published per commit, so the head advance here is a no-op and only
// the block barrier runs.
func (e *Engine) Sync(ctx context.Context) (bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/sync")
	defer task.End()

	locked := e.bcast.Lock()
	defer locked.Unlock()
	if e.closed {
		return false, ErrEngineClosed
	}

	// block barrier: drain and fence every buffered block write durable.
	fenced, err := e.writeBlockStore.Sync(ctx)
	if err != nil {
		return false, err
	}

	// advance the durable head, ordered after the block barrier. Skipped in
	// coordinator mode, where each commit already published the head.
	if e.writeCoordinator == nil && e.commitFn != nil {
		cur := e.head.root.GetRef()
		if e.durableHeadRef == nil || !e.durableHeadRef.EqualsRef(cur) {
			// The root's blocks are durable now (post-barrier); validate it is
			// followable from durable storage before publishing the head. In the
			// single-writer path this is the only validation point, because the
			// per-commit path defers both validation and the head to Sync.
			if err := e.validateRootRefLocked(ctx, cur); err != nil {
				return false, err
			}
			if err := e.commitFn(ctx, e.durableHeadRef, cur.Clone()); err != nil {
				return false, err
			}
			e.durableHeadRef = cur.Clone()
		}
	}
	return fenced, nil
}

// GetGCJournalEntries returns the number of pending GC journal entries.
// Safe to call concurrently. Returns 0 if the read tx or journal is not initialized.
func (e *Engine) GetGCJournalEntries() uint64 {
	locked := e.bcast.Lock()
	var rtx *Tx
	if e.head != nil {
		rtx = e.head.readTx
	}
	locked.Unlock()
	if rtx == nil {
		return 0
	}
	return rtx.state.GetGCJournalEntries()
}

// SetRootRef updates the root cursor to point to a new reference.
// Re-creates the internal read transaction with the updated state.
// Cancels any ongoing write tx (to be re-created against new state).
// Can return an error to indicate validation failure.
func (e *Engine) SetRootRef(ctx context.Context, ref *bucket.ObjectRef) error {
	locked := e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		return ErrEngineClosed
	}
	retirement, err := e.setRootRefLocked(ctx, ref)
	locked.Unlock()
	e.drainRetirement(ctx, retirement)
	return err
}

// AdoptRootRefFromWatch updates the root from an advisory coordinator watch
// only when no local write transaction is active. bbolt emits generation events
// for intermediate block writes before the durable World head is updated; watch
// adoption must not roll an in-flight local writer back to the previous head.
func (e *Engine) AdoptRootRefFromWatch(ctx context.Context, ref *bucket.ObjectRef) error {
	locked := e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		return ErrEngineClosed
	}
	if e.writeTx != nil {
		locked.Unlock()
		return nil
	}
	currentSeqno, ok, err := e.currentRootSeqnoLocked(ctx)
	if err != nil {
		locked.Unlock()
		return err
	}
	if ok {
		candidateSeqno, err := e.seqnoForRootRefLocked(ctx, ref)
		if err != nil {
			locked.Unlock()
			return err
		}
		if candidateSeqno < currentSeqno {
			locked.Unlock()
			return nil
		}
	}
	retirement, err := e.setRootRefLocked(ctx, ref)
	locked.Unlock()
	e.drainRetirement(ctx, retirement)
	return err
}

// setRootRefLocked updates the root reference while bcast is locked.
func (e *Engine) setRootRefLocked(
	ctx context.Context,
	ref *bucket.ObjectRef,
) (engineRetirement, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref")
	defer task.End()

	// Ignore an update that would not change the published head.
	if e.head.root.GetRef().EqualsRef(ref) {
		return engineRetirement{}, nil
	}
	// Validate the reference before building replacement state.
	if err := ref.Validate(); err != nil {
		return engineRetirement{}, err
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/follow-ref")
	nextRoot, err := e.baseRoot.FollowRef(taskCtx, ref)
	subtask.End()
	if err != nil {
		return engineRetirement{}, err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/build-world-state")
	nextWorld, err := e.buildWorldStateForRoot(taskCtx, true, nextRoot, nil)
	subtask.End()
	if err != nil {
		nextRoot.Release()
		return engineRetirement{}, err
	}

	// Publish one complete head and detach its predecessor for post-unlock cleanup.
	retirement := engineRetirement{head: e.head}
	if e.writeTx != nil {
		writeRetirement := e.writeTx.detachLocked()
		retirement.readTx = writeRetirement.readTx
		retirement.writeTx = writeRetirement.writeTx
		retirement.lease = writeRetirement.lease
		retirement.writeTxRel = writeRetirement.writeTxRel
	}
	e.head = &engineHead{
		root:   nextRoot,
		readTx: NewTx(nextWorld),
	}
	return e.beginRetirementLocked(retirement), nil
}

// beginRetirementLocked registers detached resources that Close must join.
func (e *Engine) beginRetirementLocked(retirement engineRetirement) engineRetirement {
	if !retirement.empty() {
		e.retiring++
	}
	return retirement
}

func (e *Engine) invalidateHeadReadTxLocked() engineRetirement {
	if e.head == nil || e.head.readTx == nil {
		return engineRetirement{}
	}
	retirement := e.beginRetirementLocked(engineRetirement{readTx: e.head.readTx})
	e.head = &engineHead{root: e.head.root}
	return retirement
}

// drainRetirement waits for detached transaction users before releasing their
// cursor, coordinator, and writer authorities.
func (e *Engine) drainRetirement(ctx context.Context, retirement engineRetirement) {
	if retirement.empty() {
		return
	}
	// Drain transaction users before releasing the authorities that back them.
	if retirement.writeTx != nil {
		retirement.writeTx.Discard()
	}
	if retirement.readTx != nil && retirement.readTx != retirement.writeTx {
		retirement.readTx.Discard()
	}
	if retirement.head != nil &&
		retirement.head.readTx != nil &&
		retirement.head.readTx != retirement.writeTx &&
		retirement.head.readTx != retirement.readTx {
		retirement.head.readTx.Discard()
	}
	// Release the root authority only after every coupled transaction drains.
	if retirement.head != nil && retirement.head.root != nil {
		retirement.head.root.Release()
	}
	// Release external serialization in coordinator-then-writer order.
	if retirement.lease != nil {
		_ = retirement.lease.Release(ctx)
	}
	if retirement.writeTxRel != nil {
		retirement.writeTxRel()
	}

	// Publish completion after all detached resources have been released.
	locked := e.bcast.Lock()
	e.retiring--
	locked.Broadcast()
	locked.Unlock()
}

// finishCommit publishes completion after coordinator commit cleanup finishes.
func (e *Engine) finishCommit() {
	locked := e.bcast.Lock()
	e.committing--
	locked.Broadcast()
	locked.Unlock()
}

// validateRootRefLocked rebuilds the root's WorldState without publishing it.
// Durable head publication must not happen before a fresh cursor can follow the
// full root graph outside the committing transaction.
func (e *Engine) validateRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/validate-root-ref")
	defer task.End()

	ws, err := e.worldStateForRootRefLocked(ctx, ref)
	if err != nil {
		return err
	}
	if _, err := ws.objTree.Size(ctx); err != nil {
		ws.Discard()
		return err
	}
	ws.Discard()
	return nil
}

func (e *Engine) seqnoForRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) (uint64, error) {
	ws, err := e.worldStateForRootRefLocked(ctx, ref)
	if err != nil {
		return 0, err
	}
	defer ws.Discard()
	return ws.GetSeqno(ctx)
}

func (e *Engine) worldStateForRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) (*WorldState, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	nextRoot, err := e.baseRoot.FollowRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer nextRoot.Release()

	store := nextRoot.GetBucket()
	xfrm := nextRoot.GetTransformer()
	_, bcs := nextRoot.BuildTransactionWithStore(nil, store)
	ws, err := NewWorldState(
		ctx,
		e.le,
		false,
		nil,
		bcs,
		store,
		xfrm,
		nil,
		e,
		e.lookupOp,
		e.verbose,
	)
	if err != nil {
		return nil, err
	}
	return ws, nil
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	return e.NewBlockEngineTransaction(ctx, write)
}

// NewBlockEngineTransaction returns the world-block specific EngineTx type.
func (e *Engine) NewBlockEngineTransaction(ctx context.Context, write bool) (*EngineTx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction")
	defer task.End()

	// Read-only transactions share the Engine head unless coordinator mode needs
	// a dedicated snapshot.
	if !write {
		var headRef *bucket.ObjectRef
		// Refresh the durable coordinator head before pinning local state.
		if e.writeHeadRefresh != nil {
			locked := e.bcast.Lock()
			shouldRefresh := !e.closed && e.writeTx == nil
			locked.Unlock()
			if shouldRefresh {
				var err error
				headRef, err = e.writeHeadRefresh(ctx)
				if err != nil {
					var retirement engineRetirement
					if isCoordinatedWriteSnapshotError(err) {
						locked := e.bcast.Lock()
						if !e.closed {
							retirement = e.invalidateHeadReadTxLocked()
						}
						locked.Unlock()
					}
					e.drainRetirement(ctx, retirement)
					return nil, err
				}
			}
		}

		// Revalidate Engine state after the external head refresh.
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return nil, ErrEngineClosed
		}
		var retirement engineRetirement
		var err error
		if e.writeTx == nil {
			retirement, err = e.applyDurableHeadLocked(ctx, headRef)
			if err != nil {
				locked.Unlock()
				e.drainRetirement(ctx, retirement)
				return nil, err
			}
		}
		// Uncoordinated readers use the shared head through performOp retries.
		if e.writeCoordinator == nil {
			engTx := newEngineTx(e, nil)
			locked.Unlock()
			e.drainRetirement(ctx, retirement)
			return engTx, nil
		}

		// Coordinator readers own a dedicated snapshot tracked for Engine.Close.
		world, err := e.buildWorldState(ctx, true)
		if err != nil {
			locked.Unlock()
			e.drainRetirement(ctx, retirement)
			return nil, err
		}
		engTx := newEngineTx(e, nil)
		engTx.readTx = NewTx(world)
		e.coordinatorTxs[engTx] = struct{}{}
		locked.Unlock()
		e.drainRetirement(ctx, retirement)
		return engTx, nil
	}

	// Serialize writers until Commit or Discard drains the prior transaction.
	relLock, err := e.wmtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	// Acquire and refresh the coordinator generation before reading its head.
	var lease coord.WriteLease
	if e.writeCoordinator != nil {
		lease, err = e.writeCoordinator.WaitAcquireWriteLease(ctx, e.writeCoordScope)
		if err != nil {
			relLock()
			return nil, err
		}
		if lease == nil {
			relLock()
			return nil, errors.New("world write coordinator returned nil lease")
		}
		if _, err := lease.Refresh(ctx); err != nil {
			_ = lease.Release(ctx)
			relLock()
			return nil, err
		}
	}

	// Read the durable head before entering Engine publication.
	var headRef *bucket.ObjectRef
	if e.writeHeadRefresh != nil {
		headRef, err = e.writeHeadRefresh(ctx)
		if err != nil {
			if lease != nil {
				_ = lease.Release(ctx)
			}
			relLock()
			return nil, err
		}
	}

	// Revalidate closure and adopt the durable head under the Engine lock.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction/build-world-state")
	locked := e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		if lease != nil {
			_ = lease.Release(ctx)
		}
		relLock()
		return nil, ErrEngineClosed
	}

	retirement, err := e.applyDurableHeadLocked(taskCtx, headRef)
	if err != nil {
		locked.Unlock()
		e.drainRetirement(ctx, retirement)
		if lease != nil {
			_ = lease.Release(ctx)
		}
		relLock()
		if lease != nil && isCoordinatedWriteSnapshotError(err) {
			return nil, errors.Wrap(coord.ErrStaleGeneration, "refresh durable head")
		}
		return nil, err
	}
	// Pin the base head used by commit validation while building write state.
	baseHeadRef := e.head.root.GetRef().Clone()
	world, err := e.buildWorldState(taskCtx, false)
	subtask.End()
	if err != nil {
		locked.Unlock()
		e.drainRetirement(ctx, retirement)
		if lease != nil {
			_ = lease.Release(ctx)
		}
		relLock()
		if lease != nil && isCoordinatedWriteSnapshotError(err) {
			return nil, errors.Wrapf(coord.ErrStaleGeneration, "build world state: %v", err)
		}
		return nil, err
	}

	// Assign Engine.writeTx only after the EngineTx carries its base head ref
	// and lease.
	engTx := newEngineTx(e, NewTx(world))
	engTx.baseHeadRef = baseHeadRef
	engTx.lease = lease
	e.writeTx = engTx
	e.writeTxRel = relLock
	locked.Unlock()
	e.drainRetirement(ctx, retirement)
	return engTx, nil
}

func (e *Engine) applyDurableHeadLocked(
	ctx context.Context,
	headRef *bucket.ObjectRef,
) (engineRetirement, error) {
	if headRef == nil || headRef.GetRootRef().GetEmpty() {
		return engineRetirement{}, nil
	}
	currentSeqno, ok, err := e.currentRootSeqnoLocked(ctx)
	if err != nil {
		return engineRetirement{}, err
	}
	if ok {
		candidateSeqno, err := e.seqnoForRootRefLocked(ctx, headRef)
		if err != nil {
			return engineRetirement{}, err
		}
		if candidateSeqno < currentSeqno {
			return engineRetirement{}, nil
		}
	}
	return e.setRootRefLocked(ctx, headRef)
}

func (e *Engine) currentRootSeqnoLocked(ctx context.Context) (uint64, bool, error) {
	ref := e.head.root.GetRef()
	if ref == nil || ref.GetRootRef().GetEmpty() {
		return 0, false, nil
	}
	seqno, err := e.seqnoForRootRefLocked(ctx, ref)
	return seqno, true, err
}

// ForkBlockTransaction forks the transaction at the current state.
func (e *Engine) ForkBlockTransaction(ctx context.Context, write bool) (*Tx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction")
	defer task.End()

	_, subtask := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/read-lock")
	locked := e.bcast.Lock()
	subtask.End()
	defer locked.Unlock()
	if e.closed {
		return nil, ErrEngineClosed
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/build-world-state")
	// Buffer nested and root block writes per fork so they drain in one batch
	// at Sync. Coordinator mode stays durable-on-write: its commit path
	// validates the committed root against raw storage before publication.
	store := block.StoreOps(nil)
	if write && e.writeCoordinator == nil {
		store = block.NewBufferedStore(ctx, e.writeBlockStore)
	}
	ws, err := e.buildWorldStateForRoot(taskCtx, !write, e.head.root, store)
	subtask.End()
	if err != nil {
		return nil, err
	}
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/new-tx")
	tx := NewTx(ws)
	subtask.End()
	return tx, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *Engine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	locked := e.bcast.Lock()
	defer locked.Unlock()
	if e.closed {
		return nil, ErrEngineClosed
	}
	ncs := e.baseRoot.Clone()
	ncs.SetRootRef(nil)
	return ncs, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
// The root clone is made while bcast is held for the documented same-bucket
// root path. Cursor.Clone copies the bucket handle without taking a release
// func, so this does not establish a cross-bucket lifetime across Engine.Close.
//
// NOTE: this is the implementation of AccessWorldState for the world/block engine.
func (e *Engine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	locked := e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		return ErrEngineClosed
	}
	ncs := e.head.root.Clone()
	locked.Unlock()
	defer ncs.Release()

	if ref == nil {
		return cb(ncs)
	}

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// follow the root block ref outside the engine lock
	followed, err := ncs.FollowRef(subCtx, ref)
	if err != nil {
		return err
	}
	defer followed.Release()

	return cb(followed)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *Engine) GetSeqno(ctx context.Context) (uint64, error) {
	for {
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return 0, ErrEngineClosed
		}
		readTx := e.head.readTx
		locked.Unlock()
		if readTx == nil {
			locked := e.bcast.Lock()
			if e.closed {
				locked.Unlock()
				return 0, ErrEngineClosed
			}
			seqno, ok, err := e.currentRootSeqnoLocked(ctx)
			locked.Unlock()
			if err != nil || ok {
				return seqno, err
			}
			return 0, nil
		}

		seqno, err := readTx.GetSeqno(ctx)
		if readTx.state.discarded.Load() {
			continue
		}
		return seqno, err
	}
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *Engine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	if e.writeCoordinator != nil {
		// In coordinator mode the local read snapshot does not advance on its
		// own; new seqnos appear only after other writers commit. Wait on the
		// coordinator's commit-generation events (BroadcastChannel-backed in the
		// browser) rather than polling: any seqno advance is a commit that bumps
		// the generation, so re-checking GetSeqno on each event is miss-free.
		// Baseline at the current generation so a commit racing watch setup is
		// still delivered.
		snapshot, err := e.writeCoordinator.Snapshot(ctx, e.writeCoordScope)
		if err != nil {
			return 0, err
		}
		watch, err := e.writeCoordinator.Watch(ctx, e.writeCoordScope, snapshot.Generation)
		if err != nil {
			return 0, err
		}
		defer watch.Close()
		for {
			seqno, err := e.GetSeqno(ctx)
			if err != nil {
				return 0, err
			}
			if seqno >= value {
				return seqno, nil
			}
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case _, ok := <-watch.Events():
				if !ok {
					// The coordinator closes the events channel only when this
					// wait's context is canceled, so report cancellation rather
					// than a spurious watch error.
					if err := ctx.Err(); err != nil {
						return 0, err
					}
					return 0, errors.New("world write coordinator watch closed")
				}
			}
		}
	}

	for {
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return 0, ErrEngineClosed
		}
		readTx := e.head.readTx
		locked.Unlock()

		seqno, err := readTx.WaitSeqno(ctx, value)
		if readTx.state.discarded.Load() {
			// readTxn was discarded, get the new one.
			continue
		}
		if err != nil {
			return 0, err
		}

		if seqno >= value {
			return seqno, nil
		}
	}
}

// Close releases root cursors and active transaction state owned by the engine.
func (e *Engine) Close() error {
	if e == nil {
		return nil
	}

	locked := e.bcast.Lock()
	if e.closed {
		for e.baseRoot != nil {
			wait := locked.WaitCh()
			locked.Unlock()
			<-wait
			locked = e.bcast.Lock()
		}
		locked.Unlock()
		return nil
	}
	e.closed = true

	// Detach every Engine-owned resource while publication is closed.
	retirements := make([]engineRetirement, 0, len(e.coordinatorTxs)+1)
	retirement := engineRetirement{head: e.head}
	e.head = nil
	if e.writeTx != nil {
		writeRetirement := e.writeTx.detachLocked()
		retirement.readTx = writeRetirement.readTx
		retirement.writeTx = writeRetirement.writeTx
		retirement.lease = writeRetirement.lease
		retirement.writeTxRel = writeRetirement.writeTxRel
	}
	if !retirement.empty() {
		retirements = append(retirements, e.beginRetirementLocked(retirement))
	}
	for tx := range e.coordinatorTxs {
		txRetirement := e.beginRetirementLocked(tx.detachLocked())
		if !txRetirement.empty() {
			retirements = append(retirements, txRetirement)
		}
	}
	locked.Broadcast()
	locked.Unlock()

	// Drain transaction and coordinator work after unlocking the Engine.
	for _, retirement := range retirements {
		e.drainRetirement(context.Background(), retirement)
	}

	// Join detached work and in-flight commit publication before baseRoot release.
	locked = e.bcast.Lock()
	for e.retiring != 0 || e.committing != 0 {
		wait := locked.WaitCh()
		locked.Unlock()
		<-wait
		locked = e.bcast.Lock()
	}
	// Release the block-store authority only after every dependent user exits.
	baseRoot := e.baseRoot
	locked.Unlock()
	if baseRoot != nil {
		baseRoot.Release()
	}

	// Publish final Close completion for concurrent callers.
	locked = e.bcast.Lock()
	e.baseRoot = nil
	locked.Broadcast()
	locked.Unlock()
	return nil
}

// initializeHeadReadTx constructs the shared read transaction before the
// Engine escapes from NewEngine.
func (e *Engine) initializeHeadReadTx(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/initialize-head-read-tx")
	defer task.End()

	// Keep the initialized transaction when it already matches the head.
	if e.head.readTx != nil &&
		e.head.readTx.state.GetRootRef().EqualsRef(e.head.root.GetRef().GetRootRef()) {
		return nil
	}

	// Build and publish the initial shared read transaction as one head.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/update-read-write-txns/build-world-state")
	world, err := e.buildWorldState(taskCtx, true)
	subtask.End()
	if err != nil {
		return err
	}
	e.head = &engineHead{
		root:   e.head.root,
		readTx: NewTx(world),
	}
	return nil
}

// buildWorldState builds the world state transaction and cursor fields.
// The caller must hold bcast.
func (e *Engine) buildWorldState(ctx context.Context, readOnly bool) (*WorldState, error) {
	return e.buildWorldStateForRoot(ctx, readOnly, e.head.root, nil)
}

func (e *Engine) buildWorldStateForRoot(
	ctx context.Context,
	readOnly bool,
	root *bucket_lookup.Cursor,
	transactionStore block.StoreOps,
) (*WorldState, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state")
	defer task.End()

	_, subtask := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/get-bucket")
	// Both read and write transactions read and write through writeBlockStore so
	// that, in the single-writer deferred path, reads see blocks that are
	// committed in memory but not yet drained to durable storage (read your
	// writes before Sync). It is the bucket store directly in coordinator and
	// self-buffered modes.
	store := transactionStore
	if store == nil {
		store = e.writeBlockStore
	}
	worldStore := store
	if !readOnly && e.writeCoordinator != nil {
		worldStore = nil
	}
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/get-transformer")
	xfrm := root.GetTransformer()
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/build-transaction")
	btx, bcs := root.BuildTransactionWithStore(nil, store)
	subtask.End()
	if readOnly {
		btx = nil
	}
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/new-world-state")
	ws, err := NewWorldState(
		taskCtx,
		e.le,
		!readOnly,
		btx, bcs,
		worldStore,
		xfrm,
		nil,
		e,
		e.lookupOp,
		e.verbose,
	)
	subtask.End()
	if err != nil {
		return nil, err
	}
	return ws, nil
}

// _ is a type assertion
var _ world.Engine = (*Engine)(nil)
