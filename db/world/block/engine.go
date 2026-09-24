package world_block

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// engineHead pairs a published root with the read transaction built from it.
type engineHead struct {
	// root retains the cursor authority backing this head.
	root *bucket_lookup.Cursor
	// readTx reads this root until retirement drains its users.
	readTx *Tx
}

// engineRetirement is an obsolete resource set detached from Engine publication.
// The Engine drains it after unlocking so cleanup can reenter Engine state.
type engineRetirement struct {
	// head is a replaced root and its shared read transaction.
	head *engineHead
	// readTx is a detached caller-owned read snapshot.
	readTx *Tx
	// writeTx is the detached mutable transaction.
	writeTx *Tx
	// lease retains external write authority until transaction users drain.
	lease coord.WriteLease
	// writeTxRel releases the local writer lock after the transaction drains.
	writeTxRel func()
	// session retires shared publication authority after its last user drains.
	session *engineWriteSession
}

// empty reports whether there are no detached resources to drain.
func (r engineRetirement) empty() bool {
	return r.head == nil &&
		r.readTx == nil &&
		r.writeTx == nil &&
		r.lease == nil &&
		r.writeTxRel == nil &&
		r.session == nil
}

// Engine publishes paired World roots and read snapshots.
// Local read transactions retry when their shared head retires.
// Coordinated transactions pin independently releasable durable snapshots.
type Engine struct {
	// le is the logger.
	le *logrus.Entry
	// lookupOp looks up a world operation.
	lookupOp world.LookupOp
	// verbose enables verbose logging within world state.
	verbose bool
	// wmtx ensures only one write transaction is active at a time.
	wmtx csync.Mutex
	// bcast guards Engine head, transaction, lifecycle, and close state.
	bcast broadcast.Broadcast
	// baseRoot is the base root cursor to use.
	// The root cursor is derived with FollowRef from this cursor.
	baseRoot *bucket_lookup.Cursor
	// head pairs the root cursor with its shared read transaction.
	head *engineHead
	// headChanged closes when the published head root changes, head observation
	// fails, or the Engine closes. Revision waiters wait on it instead of bcast,
	// which also wakes on every transaction retirement. Guarded by bcast.
	headChanged chan struct{}
	// writeTx is the current write transaction, canceled if its head changes.
	writeTx *EngineTx
	// writeTxRel releases wmtx after the detached write transaction drains.
	writeTxRel func()
	// snapshotTxs tracks immutable read snapshots owned by caller-held EngineTx values.
	snapshotTxs map[*EngineTx]struct{}
	// retiring counts detached resource sets that have not finished draining.
	retiring int
	// committing counts write commits that own coordinator cleanup outside bcast.
	committing int
	// commitFn is a function to be called just before a commit is confirmed.
	// It can be nil.
	commitFn        CommitFn
	atomicPublisher block.AtomicPublisher
	atomicHeadFn    AtomicHeadFn
	// stagedStore retains immutable construction bytes independently of attempts.
	stagedStore *block.BufferedStore
	// writeSession owns provisional revision ordering without changing head.
	writeSession *engineWriteSession
	// submitted is the most recently admitted completion, used as a Sync fence.
	submitted *block.PublicationReceipt
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
	// writeHeadRefresh rereads the durable World head before transactions and revision reads.
	writeHeadRefresh func(context.Context) (*bucket.ObjectRef, error)
	// headWatchCancel stops the engine-owned external head subscription.
	headWatchCancel context.CancelFunc
	// headWatchDone closes after the subscription and its refresh have drained.
	headWatchDone chan struct{}
	// headWatchErr is the latest refresh failure, guarded by bcast.
	headWatchErr error
	// closed rejects new operations while Close drains resources.
	closed bool
}

// CommitFn is a function to call with the updated root before confirming it.
// Should be used to write the updated state back to storage.
// The Engine state guard is held while commitFn runs; do not call Engine methods.
// If an error is returned the change will be rolled back.
// Do not mutate the supplied references during this call.
type CommitFn func(ctx context.Context, baseRef, nref *bucket.ObjectRef) error

// AtomicHeadFn constructs a side-effect-free head CAS for one candidate root.
type AtomicHeadFn func(baseRef, nref *bucket.ObjectRef) *block.AtomicHeadUpdate

// WithAtomicPublication enables bounded implicit staging and atomic publication
// for a coordinated engine whose metadata is in the publisher's volume. The
// caller must verify the volume identity; unsupported stores keep the old path.
func WithAtomicPublication(publisher block.AtomicPublisher, headFn AtomicHeadFn) EngineOption {
	return func(e *Engine) { e.atomicPublisher = publisher; e.atomicHeadFn = headFn }
}

// EngineOption configures optional Engine integrations.
type EngineOption func(*Engine)

// WithDeferredDurability enables single-writer deferred durability: block writes
// and the durable head advance batch in memory until Sync.
// It has no effect when a write coordinator is configured.
func WithDeferredDurability() EngineOption {
	return func(e *Engine) {
		e.deferDurability = true
	}
}

// WithWriteCoordinator requires write transactions to acquire a coordinator
// lease and refresh the durable World head before mutation.
// Read transactions and revision reads also refresh the durable head.
// The engine watches external commits until Close. refreshHead must support
// concurrent calls from the watcher and transactions.
func WithWriteCoordinator(
	coordinator coord.Coordinator,
	scope coord.Scope,
	keyPrefix []byte,
	refreshHead func(context.Context) (*bucket.ObjectRef, error),
) EngineOption {
	return func(e *Engine) {
		e.writeCoordinator = coordinator
		e.writeCoordScope = scope
		e.writeCoordKeyPrefix = slices.Clone(keyPrefix)
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
	// Construct the publication state before applying optional integrations.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new")
	defer task.End()

	// Keep the base cursor and published read head as separate authorities.
	e := &Engine{
		le:             le,
		baseRoot:       root,
		lookupOp:       lookupOp,
		head:           &engineHead{root: root.Clone()},
		headChanged:    make(chan struct{}),
		commitFn:       commitFn,
		durableHeadRef: root.GetRef().Clone(),
		verbose:        verbose,
		snapshotTxs:    make(map[*EngineTx]struct{}),
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

	if e.writeCoordinator != nil && e.atomicPublisher != nil && e.atomicHeadFn != nil && e.atomicPublisher.SupportsAtomicPublication() {
		// Content-addressed bytes are engine-retained, not transaction authority.
		// A discarded attempt can leave a cursor or ref in use by a retry. Keep
		// that content readable; capacity pressure prepares it durably without
		// publishing a head. No state proportional to unrelated history is kept.
		e.stagedStore = block.NewBufferedStoreWithSettings(ctx, rawWriteStore,
			&block.BufferedStoreSettings{MaxPendingBytes: 4 << 20, MaxPendingMetadataBytes: 4 << 20, MaxPendingEntries: 4096})
	}

	// Publish a complete read head before returning the engine.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new/initialize-head-read-tx")
	err := e.initializeHeadReadTx(taskCtx)
	subtask.End()
	if err != nil {
		// Close only releases local resources and cannot fail here.
		_ = e.Close()
		return nil, err
	}

	// External head observation belongs to the engine's resource lifetime.
	// Constructor cancellation does not invalidate retained engine handles.
	if e.writeCoordinator != nil && e.writeHeadRefresh != nil {
		watchCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		e.headWatchCancel = cancel
		e.headWatchDone = make(chan struct{})
		go e.watchCoordinatorHead(watchCtx)
	}
	return e, nil
}

// GetRootRef gets the current root cursor reference.
func (e *Engine) GetRootRef() *bucket.ObjectRef {
	locked := e.bcast.Lock()
	var ref *bucket.ObjectRef
	if e.head != nil {
		ref = e.head.root.GetRef().Clone()
	}
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
	// Hold publication stable across the durability fence.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/sync")
	defer task.End()

	// Join the admission watermark outside the publication guard: completion
	// needs that guard to install the durable head. Later submissions need not
	// complete for this fence.
	locked := e.bcast.Lock()
	submitted := e.submitted
	locked.Unlock()
	if submitted != nil {
		if err := submitted.Wait(ctx); err != nil {
			return false, err
		}
	}
	// Hold the published root stable across the backing-store fence.
	locked = e.bcast.Lock()
	defer locked.Unlock()
	if e.closed {
		return false, ErrEngineClosed
	}

	// Construction cursors write into the engine-retained overlay even before
	// a World transaction is submitted. Fence those bytes as durable preparation
	// as well, without changing a head. Completed borrows are returned before
	// their completion goroutine needs bcast, so this cannot block publication.
	store := e.writeBlockStore
	if e.stagedStore != nil {
		store = e.stagedStore
	}
	fenced, err := store.Sync(ctx)
	if err != nil {
		return false, err
	}

	// Advance the durable head, ordered after the block barrier. Skipped in
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
	// Accepted publications own revision authority until their receipts settle.
	// Wait outside bcast, then recheck under the same guard before replacing it.
	locked := e.bcast.Lock()
	for !e.closed && e.writeSession != nil && e.writeSession.pending != 0 {
		wait := locked.WaitCh()
		locked.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait:
		}
		locked = e.bcast.Lock()
	}
	if e.closed {
		locked.Unlock()
		return ErrEngineClosed
	}
	retirement, err := e.setRootRefLocked(ctx, ref)
	locked.Unlock()

	// Retired readers may reenter the engine during cleanup.
	e.drainRetirement(ctx, retirement)
	return err
}

// AdoptRootRefFromWatch updates the root from an advisory coordinator watch
// only when no local write transaction is active. bbolt emits generation events
// for intermediate block writes before the durable World head is updated; watch
// adoption must not roll an in-flight local writer back to the previous head.
func (e *Engine) AdoptRootRefFromWatch(ctx context.Context, ref *bucket.ObjectRef) error {
	// Ignore advisory updates while a local writer holds publication authority.
	locked := e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		return ErrEngineClosed
	}
	if e.writeTx != nil || e.writeSession != nil {
		locked.Unlock()
		return nil
	}

	// An identical DAG root already has the published revision and state.
	if e.head.root.GetRef().EqualsRef(ref) {
		locked.Unlock()
		return nil
	}

	// Reject an advisory root older than the published revision.
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

	// Drain replaced state after releasing publication authority.
	e.drainRetirement(ctx, retirement)
	return err
}

// setRootRefLocked updates the root reference while bcast is locked.
func (e *Engine) setRootRefLocked(
	ctx context.Context,
	ref *bucket.ObjectRef,
) (engineRetirement, error) {
	return e.installRootRefLocked(ctx, ref, true)
}

// installRootRefLocked can preserve the active provisional successor. Durable
// completion of N must never discard the already preparing N+1 transaction.
func (e *Engine) installRootRefLocked(ctx context.Context, ref *bucket.ObjectRef, invalidateWriter bool) (engineRetirement, error) {
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

	// Build a complete replacement before detaching the published head.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/follow-ref")
	nextRoot, err := e.baseRoot.FollowRef(taskCtx, ref)
	subtask.End()
	if err != nil {
		return engineRetirement{}, err
	}

	// Construct the replacement read transaction before changing the head.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/build-world-state")
	nextWorld, err := e.buildWorldStateForRoot(taskCtx, true, nextRoot, nil)
	subtask.End()
	if err != nil {
		nextRoot.Release()
		return engineRetirement{}, err
	}

	// Publish one complete head and detach its predecessor for post-unlock cleanup.
	retirement := engineRetirement{head: e.head}
	if invalidateWriter && e.writeTx != nil {
		writeRetirement := e.writeTx.detachLocked()
		retirement.readTx = writeRetirement.readTx
		retirement.writeTx = writeRetirement.writeTx
		retirement.lease = writeRetirement.lease
		retirement.writeTxRel = writeRetirement.writeTxRel
		retirement.session = writeRetirement.session
	}
	e.head = &engineHead{
		root:   nextRoot,
		readTx: NewTx(nextWorld),
	}
	e.signalHeadLocked()
	return e.beginRetirementLocked(retirement), nil
}

// signalHeadLocked wakes revision waiters. The caller must hold bcast.
func (e *Engine) signalHeadLocked() {
	close(e.headChanged)
	e.headChanged = make(chan struct{})
}

// beginRetirementLocked registers detached resources that Close must join.
func (e *Engine) beginRetirementLocked(retirement engineRetirement) engineRetirement {
	if !retirement.empty() {
		e.retiring++
	}
	return retirement
}

// invalidateHeadReadTxLocked detaches an invalid shared snapshot under bcast.
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
func (e *Engine) drainRetirement(ctx context.Context, retirement engineRetirement) error {
	if retirement.empty() {
		return nil
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
	var cleanupErr error
	if retirement.lease != nil {
		cleanupErr = retirement.lease.Release(context.WithoutCancel(ctx))
	}
	if retirement.session != nil {
		if err := e.releaseWriteSession(retirement.session); cleanupErr == nil {
			cleanupErr = err
		}
	}
	if retirement.writeTxRel != nil {
		retirement.writeTxRel()
	}

	// Publish completion after all detached resources have been released.
	locked := e.bcast.Lock()
	e.retiring--
	locked.Broadcast()
	locked.Unlock()
	return cleanupErr
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

	// Verify the candidate graph can be read from its durable store.
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

// seqnoForRootRefLocked reads a candidate root's revision without publishing it.
func (e *Engine) seqnoForRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) (uint64, error) {
	ws, err := e.worldStateForRootRefLocked(ctx, ref)
	if err != nil {
		return 0, err
	}
	defer ws.Discard()
	return ws.GetSeqno(ctx)
}

// worldStateForRootRefLocked builds independently releasable state for a candidate root.
func (e *Engine) worldStateForRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) (*WorldState, error) {
	// Resolve the candidate root without changing Engine publication.
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	nextRoot, err := e.baseRoot.FollowRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer nextRoot.Release()

	// Give the candidate WorldState its own storage transaction.
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

	// Read-only transactions retain one immutable World revision until Discard.
	if !write {
		if err := e.refreshReadHead(ctx); err != nil {
			return nil, err
		}

		// Pin the current head after refresh and retirement complete.
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return nil, ErrEngineClosed
		}

		// Each reader owns a dedicated snapshot tracked for Engine.Close.
		world, err := e.buildWorldState(ctx, true)
		if err != nil {
			locked.Unlock()
			return nil, err
		}
		engTx := newEngineTx(e, nil)
		engTx.readTx.Store(NewTx(world))
		engTx.readRoot = e.head.root.GetRef().Clone()
		e.snapshotTxs[engTx] = struct{}{}
		locked.Unlock()
		return engTx, nil
	}

	// Serialize writers until Commit or Discard drains the prior transaction.
	relLock, err := e.wmtx.Lock(ctx)
	if err != nil {
		return nil, err
	}

	// Reuse the one revision authority while prior durable finishes are pending.
	// Only this producer sees the private head; ordinary readers still use head.
	if e.stagedStore != nil {
		engTx, resumed, resumeErr := e.resumeWriteSession(ctx, relLock)
		if resumed || resumeErr != nil {
			if resumeErr != nil {
				relLock()
			}
			return engTx, resumeErr
		}
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

	// Apply the lease-refreshed head before constructing mutable state.
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
	var world *WorldState
	if e.stagedStore != nil {
		world, err = e.buildWorldStateForRoot(taskCtx, false, e.head.root, e.stagedStore)
	} else {
		world, err = e.buildWorldState(taskCtx, false)
	}
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
	engTx.staged = e.stagedStore
	if e.stagedStore != nil {
		session := &engineWriteSession{lease: lease, prepared: baseHeadRef.Clone()}
		e.writeSession = session
		engTx.session = session
	} else {
		engTx.lease = lease
	}
	e.writeTx = engTx
	e.writeTxRel = relLock
	locked.Unlock()
	e.drainRetirement(ctx, retirement)
	return engTx, nil
}

// refreshReadHead adopts the durable head when no local writer owns publication.
// External I/O and retirement run without holding bcast.
func (e *Engine) refreshReadHead(ctx context.Context) error {
	if e.writeHeadRefresh == nil {
		return nil
	}

	// Refresh the durable coordinator head before pinning local state.
	var headRef *bucket.ObjectRef
	locked := e.bcast.Lock()
	shouldRefresh := !e.closed && e.writeTx == nil && e.writeSession == nil
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
			return err
		}
	}

	// Revalidate writer ownership after external I/O before adopting the head.
	locked = e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		return ErrEngineClosed
	}
	var retirement engineRetirement
	var err error
	if e.writeTx == nil && e.writeSession == nil {
		retirement, err = e.applyDurableHeadLocked(ctx, headRef)
	}
	locked.Unlock()
	e.drainRetirement(ctx, retirement)
	return err
}

// applyDurableHeadLocked adopts a durable root without moving the revision backwards.
func (e *Engine) applyDurableHeadLocked(
	ctx context.Context,
	headRef *bucket.ObjectRef,
) (engineRetirement, error) {
	// An absent durable head does not replace the initialized World.
	if headRef == nil || headRef.GetRootRef().GetEmpty() {
		return engineRetirement{}, nil
	}

	// Compare content identity before opening state to compare revisions.
	if e.head.root.GetRef().EqualsRef(headRef) {
		return engineRetirement{}, nil
	}

	// A distinct durable root may advance the revision but cannot roll it back.
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

// currentRootSeqnoLocked returns the published root's revision and whether it exists.
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

	// Prevent writes to the source snapshot while forking.
	_, subtask := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/read-lock")
	locked := e.bcast.Lock()
	subtask.End()
	defer locked.Unlock()
	if e.closed {
		return nil, ErrEngineClosed
	}

	// Build a distinct World state over the retained root.
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
	if write {
		release, err := block.PinRoot(ctx, e.writeBlockStore, e.head.root.GetRef().GetRootRef())
		if err != nil {
			ws.Discard()
			return nil, err
		}
		ws.readRelease = release
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
	if e.stagedStore != nil {
		ncs.SetTransactionStore(e.stagedStore)
	}
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
	// Retain the current root while publication is stable.
	locked := e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		return ErrEngineClosed
	}
	ncs := e.head.root.Clone()
	release, err := block.PinRoot(ctx, e.writeBlockStore, ncs.GetRef().GetRootRef())
	if err != nil {
		locked.Unlock()
		ncs.Release()
		return err
	}
	defer release()
	if ref != nil && e.stagedStore != nil {
		ncs.SetTransactionStore(e.stagedStore)
	}
	locked.Unlock()
	defer ncs.Release()

	// Use the current root when no explicit reference was supplied.
	if ref == nil {
		return cb(ncs)
	}

	// Follow a supplied root outside the engine lock.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	followed, err := ncs.FollowRef(subCtx, ref)
	if err != nil {
		return err
	}
	defer followed.Release()

	// Expose the followed cursor only for the callback lifetime.
	return cb(followed)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *Engine) GetSeqno(ctx context.Context) (uint64, error) {
	// Preserve durable freshness without opening a dedicated transaction.
	if err := e.refreshReadHead(ctx); err != nil {
		return 0, err
	}

	// Reuse the shared head published by local commits or durable refresh.
	for {
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return 0, ErrEngineClosed
		}
		if err := e.initializeHeadReadTx(ctx); err != nil {
			locked.Unlock()
			return 0, err
		}
		readTx := e.head.readTx
		locked.Unlock()

		// Retry a revision read if its shared head retired concurrently.
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
	// Discover durable commits that preceded this wait even if their advisory
	// notification was lost. Subsequent events refresh the shared Engine head.
	if e.writeHeadRefresh != nil {
		seqno, err := e.GetSeqno(ctx)
		if err != nil {
			return 0, err
		}
		if seqno >= value {
			return seqno, nil
		}
	}

	// Capture the head and its next publication event under the same lock.
	for {
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return 0, ErrEngineClosed
		}
		if err := e.initializeHeadReadTx(ctx); err != nil {
			locked.Unlock()
			return 0, err
		}
		readTx := e.head.readTx
		watchErr := e.headWatchErr
		wait := e.headChanged
		locked.Unlock()

		// Read the captured head without holding publication authority.
		seqno, err := readTx.GetSeqno(ctx)
		if readTx.state.discarded.Load() {
			continue
		}
		if err != nil {
			return 0, err
		}

		// Return a satisfied revision before considering observation failure.
		if seqno >= value {
			return seqno, nil
		}
		if watchErr != nil {
			return 0, watchErr
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-wait:
		}
	}
}

// Close releases root cursors and active transaction state owned by the engine.
func (e *Engine) Close() error {
	if e == nil {
		return nil
	}

	// Cancel before acquiring publication authority so an in-flight refresh can
	// release I/O and locks that Close must join.
	if e.headWatchCancel != nil {
		e.headWatchCancel()
	}

	// Close publication or join the caller already draining it.
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
	e.signalHeadLocked()

	// Detach every Engine-owned resource while publication is closed.
	retirements := make([]engineRetirement, 0, len(e.snapshotTxs)+1)
	retirement := engineRetirement{head: e.head}
	e.head = nil
	if e.writeTx != nil {
		writeRetirement := e.writeTx.detachLocked()
		retirement.readTx = writeRetirement.readTx
		retirement.writeTx = writeRetirement.writeTx
		retirement.lease = writeRetirement.lease
		retirement.writeTxRel = writeRetirement.writeTxRel
		retirement.session = writeRetirement.session
	}
	if !retirement.empty() {
		retirements = append(retirements, e.beginRetirementLocked(retirement))
	}
	for tx := range e.snapshotTxs {
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
	if e.headWatchDone != nil {
		<-e.headWatchDone
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

// initializeHeadReadTx constructs or restores the shared read transaction.
// The caller must hold bcast after construction.
func (e *Engine) initializeHeadReadTx(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/initialize-head-read-tx")
	defer task.End()

	// Keep the initialized transaction when it already matches the head.
	if e.head.readTx != nil &&
		e.head.readTx.state.GetRootRef().EqualsRef(e.head.root.GetRef().GetRootRef()) {
		return nil
	}

	// Publish the restored read transaction together with its root.
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

// buildWorldStateForRoot builds state using the engine's stores and lookup policy.
// The caller must hold bcast after construction.
func (e *Engine) buildWorldStateForRoot(
	ctx context.Context,
	readOnly bool,
	root *bucket_lookup.Cursor,
	transactionStore block.StoreOps,
) (*WorldState, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state")
	defer task.End()

	// Read the bucket under the existing transaction authority.
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
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/get-transformer")
	xfrm := root.GetTransformer()
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/build-transaction")
	btx, bcs := root.BuildTransactionWithStore(nil, store)
	subtask.End()
	releaseRoot := func() {}
	if readOnly {
		var err error
		releaseRoot, err = block.PinRoot(ctx, store, root.GetRef().GetRootRef())
		if err != nil {
			return nil, err
		}
	}

	// The published read head already holds this immutable root. Clone its
	// decoded value so each new handle owns its mutable sub-block fields.
	if e.head != nil && e.head.readTx != nil && e.head.root.GetRef().EqualsRef(root.GetRef()) &&
		e.head.readTx.state.GetRootRef().EqualsRef(root.GetRef().GetRootRef()) {
		rootBlock, err := e.head.readTx.state.GetRoot(ctx)
		if err != nil {
			releaseRoot()
			return nil, err
		}
		bcs.SetBlock(rootBlock.CloneVT(), false)
	}
	if readOnly {
		btx = nil
	} else {
		rootBlock, err := UnmarshalWorld(ctx, bcs)
		if err != nil {
			releaseRoot()
			return nil, err
		}
		if rootBlock.GetGcGraph() != nil || rootBlock.GetGcJournal() != nil {
			rootBlock = rootBlock.CloneVT()
			rootBlock.GcGraph, rootBlock.GcJournal = nil, nil
			bcs.SetBlock(rootBlock, true)
		}
	}
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/new-world-state")
	// Physical reachability belongs to the volume. Replicating a graph of
	// our own block writes cannot reclaim shared bytes and amplifies writes.
	ws, err := newWorldState(
		taskCtx,
		e.le,
		!readOnly,
		btx, bcs,
		store,
		xfrm,
		nil,
		e,
		e.lookupOp,
		e.verbose,
		false,
	)
	subtask.End()
	if err != nil {
		releaseRoot()
		return nil, err
	}
	ws.readRelease = releaseRoot
	ws.localBucketID = root.GetRefWithOpArgs().GetBucketId()
	return ws, nil
}

// Verify the World engine contract.
var _ world.Engine = (*Engine)(nil)
