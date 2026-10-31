package world_block

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// engineHead pairs a published root with the read transaction built from it.
type engineHead struct {
	// root retains the cursor authority backing this head.
	root *bucket_lookup.Cursor
	// pin keeps root's blocks from collection while the head or a holder
	// uses it.
	pin *rootPin
	// readTx reads this root until retirement drains its users.
	readTx *Tx
}

// rootPin shares one root pin among its holders. Publication pins each head
// once, so a holder takes the pin under bcast without block I/O.
type rootPin struct {
	// refs counts the holders, including the head.
	refs atomic.Int64
	// unpin releases the pin after the last holder.
	unpin func()
}

// newRootPin pins ref in store for its first holder.
func newRootPin(ctx context.Context, store block.StoreOps, ref *block.BlockRef) (*rootPin, error) {
	// Pin the root and count the first holder.
	unpin, err := block.PinRoot(ctx, store, ref)
	if err != nil {
		return nil, err
	}
	p := &rootPin{unpin: unpin}
	p.refs.Store(1)
	return p, nil
}

// hold adds a holder and returns its release.
func (p *rootPin) hold() func() {
	p.refs.Add(1)
	return p.release
}

// release drops a holder and unpins after the last one.
func (p *rootPin) release() {
	if p.refs.Add(-1) == 0 {
		p.unpin()
	}
}

// publishedHead is an immutable published head that readers load and hold
// without the Engine lock. The Engine holds one reference while the head is
// published and every reader holds another until it releases, so a reader
// keeps reading the storage of the head it pinned after a newer head
// replaces it.
type publishedHead struct {
	// refs counts the holders. Zero means retired, and a retired head is
	// never held again.
	refs atomic.Int64
	// root is a cursor on the head root, only cloned, never changed.
	root *bucket_lookup.Cursor
	// ref is the head root reference, never changed.
	ref *bucket.ObjectRef
	// unpin drops this head's hold on the root pin.
	unpin func()
	// rootBlock is the decoded World root, or nil. Callers clone it before use.
	rootBlock *World
	// seqno is the revision of the head.
	seqno uint64
}

// tryHold adds a holder unless the head has retired.
func (p *publishedHead) tryHold() bool {
	// Count up only from a live count, so a retired head stays retired.
	for {
		n := p.refs.Load()
		if n <= 0 {
			return false
		}
		if p.refs.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// hold adds a holder. The caller must already hold the head.
func (p *publishedHead) hold() func() {
	p.refs.Add(1)
	return p.release
}

// release drops a holder and releases the root after the last one.
func (p *publishedHead) release() {
	if p.refs.Add(-1) == 0 {
		p.root.Release()
		p.unpin()
	}
}

// engineRetirement is an obsolete resource set detached from Engine publication.
// The Engine drains it after unlocking so cleanup can reenter Engine state.
type engineRetirement struct {
	// head is a replaced root and its shared read transaction.
	head *engineHead
	// pub is the Engine's reference to a replaced published head.
	pub *publishedHead
	// readTx is a detached caller-owned read snapshot.
	readTx *Tx
	// readHead is the head that snapshot held for its object reads.
	readHead *heldHead
	// writeTx is the detached mutable transaction.
	writeTx *Tx
	// lease retains external write authority until transaction users drain.
	lease coord.WriteLease
	// writeTxRel releases the local writer lock after the transaction drains.
	writeTxRel func()
	// session retires shared publication authority after its last user drains.
	session *engineWriteSession
	// watched are heads no longer recorded for change watchers.
	watched []*watchedHead
}

// empty reports whether there are no detached resources to drain.
func (r engineRetirement) empty() bool {
	return r.head == nil &&
		r.pub == nil &&
		r.readTx == nil &&
		r.readHead == nil &&
		r.writeTx == nil &&
		r.lease == nil &&
		r.writeTxRel == nil &&
		r.session == nil &&
		len(r.watched) == 0
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
	// pub is the published head read transactions open at. It is swapped
	// under bcast and loaded without it, so a read open never waits on a
	// writer holding bcast. Nil after Close.
	pub atomic.Pointer[publishedHead]
	// writerOwned mirrors writeTx != nil || writeSession != nil for readers
	// that skip a durable head refresh while a local writer owns publication.
	// Stored under bcast.
	writerOwned atomic.Bool
	// snapMu guards read snapshot registration. It is never held across I/O,
	// and bcast may be taken before it but never while holding it.
	snapMu sync.Mutex
	// snapIdle wakes Close when opening or snapRetiring reaches zero.
	snapIdle *sync.Cond
	// snapClosed rejects new read snapshots once Close starts. Guarded by snapMu.
	snapClosed bool
	// snapshotTxs tracks immutable read snapshots owned by caller-held EngineTx
	// values. Guarded by snapMu.
	snapshotTxs map[*EngineTx]struct{}
	// opening counts read transaction opens building state. Guarded by snapMu.
	opening int
	// snapRetiring counts discarded read snapshots still releasing their
	// state. Guarded by snapMu.
	snapRetiring int
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
	// durableSeqno is the seqno of the durable head: every commit at or below
	// it is durable and covered by the head. In deferred-durability mode only
	// Sync advances it; otherwise a head becomes published only once durable,
	// so it follows the published head. Guarded by bcast.
	durableSeqno uint64
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
	// changeWatch compares each new head once for every watcher. It is nil
	// while no watcher is registered. Guarded by bcast.
	changeWatch *engineChangeWatch
	// changeWatches counts change watch goroutines that Close must join.
	changeWatches int
	// unknownChanges counts unknown spans from the shared change watch, under bcast.
	unknownChanges uint64
	// watchedHeads records the recent published heads, oldest first, for
	// change watchers. Guarded by bcast.
	watchedHeads []*watchedHead
	// watchedFloor is the lowest seqno whose recorded heads identify what a
	// watcher saw. Guarded by bcast.
	watchedFloor uint64
	// closed rejects new operations while Close drains resources.
	closed bool
}

// OperationAuthor returns no signer for a standalone block World.
func (e *Engine) OperationAuthor(context.Context) (peer.ID, string, error) {
	return "", "", nil
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
		watchedHeads:   make([]*watchedHead, 0, changeWatchHeads),
	}
	e.snapIdle = sync.NewCond(&e.snapMu)
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
	if e.deferredDurability() {
		if rawWriteStore.GetSupportedFeatures()&block.StoreFeatureSelfBuffered == 0 {
			// Not self-buffered (e.g. s4db): defer behind one long-lived
			// BufferedStore that accumulates writes in memory until Sync.
			e.writeBlockStore = block.NewBufferedStore(ctx, rawWriteStore)
		}
	}

	// Pin the opening head against collection.
	pin, err := newRootPin(ctx, e.writeBlockStore, e.head.root.GetRef().GetRootRef())
	if err != nil {
		e.head.root.Release()
		return nil, err
	}
	e.head.pin = pin

	// Stage writes when the engine can publish a head atomically.
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
	err = e.initializeHeadReadTx(taskCtx)
	subtask.End()
	if err == nil {
		err = e.recordInitialHead(ctx)
	}
	if err != nil {
		// Close only releases local resources and cannot fail here.
		_ = e.Close()
		return nil, err
	}
	e.durableSeqno = e.headSeqnoLocked()

	// Publish the opening head for lock-free readers.
	e.publishHeadLocked(ctx, e.durableSeqno)

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

// deferredDurability reports whether block writes and the durable head batch
// until Sync: the single-writer mode, since a coordinator stays durable on write.
func (e *Engine) deferredDurability() bool {
	return e.deferDurability && e.writeCoordinator == nil
}

// GetRootRef gets the current root cursor reference.
func (e *Engine) GetRootRef() *bucket.ObjectRef {
	// Clone the published head ref without the engine lock.
	if p := e.pub.Load(); p != nil {
		return p.ref.Clone()
	}
	return nil
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
	// needs that guard to install the durable head, and a submitter needs it
	// to retire its writer before it starts the completion that returns its
	// staged borrow. Admission takes that borrow and advances the watermark
	// under the guard together, so an unchanged watermark under the guard
	// proves no borrow is outstanding when the store fence starts.
	locked := e.bcast.Lock()
	for submitted := e.submitted; submitted != nil; submitted = e.submitted {
		locked.Unlock()
		if err := submitted.Wait(ctx); err != nil {
			return false, err
		}
		locked = e.bcast.Lock()
		if e.submitted == submitted {
			break
		}
	}

	// Hold the published root stable across the backing-store fence.
	defer locked.Unlock()
	if e.closed {
		return false, ErrEngineClosed
	}
	return e.syncHeadLocked(ctx)
}

// SyncThrough returns once every commit with seqno <= seqno is durable and
// covered by the durable head, and returns the durable seqno, >= seqno.
//
// It waits only for the published head to reach seqno. Commits admitted after
// seqno are not joined: a coordinated publication installs its head, and so
// advances the durable seqno, only after it is durable, in admission order. So
// a head at seqno needs no submission fence. In deferred-durability mode no
// submissions exist, and a head not yet covered by the durable head takes the
// same ordered fence as Sync. The shared BufferedStore flushes every pending
// block, not only those reachable from seqno, and the durable head advances to
// the published head, which can be newer than seqno.
func (e *Engine) SyncThrough(ctx context.Context, seqno uint64) (uint64, error) {
	// Trace the fence as one task.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/sync-through")
	defer task.End()

	// Wait for the head to publish seqno, unless the durable head covers it.
	for {
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return 0, ErrEngineClosed
		}
		if e.durableSeqno >= seqno {
			durable := e.durableSeqno
			locked.Unlock()
			return durable, nil
		}
		if e.headSeqnoLocked() >= seqno {
			defer locked.Unlock()
			break
		}
		locked.Unlock()
		if _, err := e.WaitSeqno(ctx, seqno); err != nil {
			return 0, err
		}
	}

	// The head holds seqno without a durable head covering it: fence now,
	// with the published head stable across the fence.
	if _, err := e.syncHeadLocked(ctx); err != nil {
		return 0, err
	}
	return e.durableSeqno, nil
}

// syncHeadLocked runs the ordered durability fence for the published head and
// records the durable seqno it reaches. The caller must hold bcast, with no
// publication borrow outstanding.
func (e *Engine) syncHeadLocked(ctx context.Context) (bool, error) {
	// Transactions write into the engine-retained overlay before they are
	// submitted. Fence those bytes as durable preparation as well, without
	// changing a head. The watermark wait above left no publication borrow
	// outstanding, so this fence waits on no goroutine that needs bcast.
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
	e.durableSeqno = max(e.durableSeqno, e.headSeqnoLocked())
	return fenced, nil
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
// only when no local write transaction is active. The coordinator emits
// generation events for intermediate block writes before the durable World
// head is updated; watch adoption must not roll an in-flight local writer back
// to the previous head.
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
	// Start the root-ref trace.
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

	// Record the replacement for change watchers.
	seqno, err := nextWorld.GetSeqno(ctx)
	if err != nil {
		nextWorld.Discard()
		nextRoot.Release()
		return engineRetirement{}, err
	}

	// Pin the replacement once for every later holder.
	pin, err := newRootPin(ctx, e.writeBlockStore, nextRoot.GetRef().GetRootRef())
	if err != nil {
		nextWorld.Discard()
		nextRoot.Release()
		return engineRetirement{}, err
	}
	var watched []*watchedHead
	if evicted := e.recordWatchedHeadLocked(nextRoot, seqno); evicted != nil {
		watched = append(watched, evicted)
	}

	// Publish one complete head and detach its predecessor for post-unlock cleanup.
	retirement := engineRetirement{head: e.head, watched: watched}
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
		pin:    pin,
		readTx: NewTx(nextWorld),
	}
	retirement.pub = e.publishHeadLocked(ctx, seqno)
	if !e.deferredDurability() {
		// A head publishes only once durable outside deferred mode.
		e.durableSeqno = max(e.durableSeqno, seqno)
	}
	e.signalHeadLocked()
	return e.beginRetirementLocked(retirement), nil
}

// publishHeadLocked publishes e.head at seqno for lock-free readers and
// returns the replaced published head, whose Engine reference the caller
// releases after unlocking. The caller must hold bcast.
func (e *Engine) publishHeadLocked(ctx context.Context, seqno uint64) *publishedHead {
	// Seed readers with the decoded root when the head has one; a reader
	// without it decodes the root itself.
	rootBlock, err := e.headRootBlockLocked(ctx)
	if err != nil {
		rootBlock = nil
	}

	// Hold the head's root pin for as long as any reader holds this head.
	p := &publishedHead{
		root:      e.head.root.Clone(),
		ref:       e.head.root.GetRef().Clone(),
		unpin:     e.head.pin.hold(),
		rootBlock: rootBlock,
		seqno:     seqno,
	}
	p.refs.Store(1)
	return e.pub.Swap(p)
}

// acquireHead holds the published head without the Engine lock, or returns
// nil after Close. A retired head was already replaced, so the retry loads a
// newer one.
func (e *Engine) acquireHead() *publishedHead {
	for {
		p := e.pub.Load()
		if p == nil || p.tryHold() {
			return p
		}
	}
}

// syncWriterOwnedLocked records whether a local writer owns publication.
// The caller must hold bcast and call it after changing writeTx or
// writeSession.
func (e *Engine) syncWriterOwnedLocked() {
	e.writerOwned.Store(e.writeTx != nil || e.writeSession != nil)
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
	e.head = &engineHead{root: e.head.root, pin: e.head.pin}
	return retirement
}

// drainRetirement waits for detached transaction users before releasing their
// cursor, coordinator, and writer authorities.
func (e *Engine) drainRetirement(ctx context.Context, retirement engineRetirement) error {
	// Return when the retirement has nothing to drain.
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
	// Readers still holding a replaced published head keep its root.
	if retirement.pub != nil {
		retirement.pub.release()
	}
	if retirement.readHead != nil {
		retirement.readHead.release()
	}
	if retirement.head != nil && retirement.head.root != nil {
		retirement.head.root.Release()
		retirement.head.pin.release()
	}
	for _, h := range retirement.watched {
		h.root.Release()
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
	// Start the root-validation trace.
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
	_, bcs := nextRoot.BuildTransactionWithStore(nil, store)
	return NewWorldState(ctx, e.le, false, nil, bcs, store, e, e.lookupOp, e.verbose)
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
	// Start the transaction trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction")
	defer task.End()

	// Read-only transactions retain one immutable World revision until Discard.
	if !write {
		if err := e.refreshReadHead(ctx); err != nil {
			return nil, err
		}

		return e.openReadTx(ctx)
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
	world, err := e.buildWriteState(taskCtx, e.head.root)
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
	e.syncWriterOwnedLocked()
	locked.Unlock()

	// Drain retired roots and return the write transaction.
	e.drainRetirement(ctx, retirement)
	return engTx, nil
}

// openReadTx opens a read snapshot of the published head without the Engine
// lock, so a read never waits on a writer holding bcast. The snapshot may lag
// behind a commit publishing concurrently. It holds the head it opened at
// until Discard, so it keeps reading that head's storage. Close joins an open
// in flight through opening.
func (e *Engine) openReadTx(ctx context.Context) (*EngineTx, error) {
	// Count the open so Close joins it, unless the Engine has closed.
	e.snapMu.Lock()
	if e.snapClosed {
		e.snapMu.Unlock()
		return nil, ErrEngineClosed
	}
	e.opening++
	e.snapMu.Unlock()

	// Hold the published head for the snapshot's object reads and for its
	// state, and build the state from it.
	var engTx *EngineTx
	var world *WorldState
	err := ErrEngineClosed
	head := e.acquireHead()
	var root *bucket_lookup.Cursor
	if head != nil {
		root = head.root.Clone()
		world, err = e.newWorldState(ctx, true, root, e.writeBlockStore, head.hold(), head.rootBlock)
	}

	// Register the snapshot for Engine.Close unless it closed meanwhile.
	e.snapMu.Lock()
	e.opening--
	closed := e.snapClosed
	if closed {
		e.snapIdle.Broadcast()
	}
	if err == nil && !closed {
		engTx = newEngineTx(e, nil)
		engTx.readTx.Store(NewTx(world))
		engTx.readRoot = head.ref.Clone()
		engTx.readHead.Store(newHeldHead(root, func() {
			root.Release()
			head.release()
		}))
		e.snapshotTxs[engTx] = struct{}{}
	}
	e.snapMu.Unlock()
	if engTx != nil {
		return engTx, nil
	}

	// Release what the failed open held.
	if world != nil {
		world.Discard()
	}
	if head != nil {
		root.Release()
		head.release()
	}
	if err == nil {
		err = ErrEngineClosed
	}
	return nil, err
}

// discardSnapshot detaches a read snapshot under snapMu and releases its
// state after unlocking, without the Engine lock.
func (e *Engine) discardSnapshot(tx *EngineTx) {
	// Detach the snapshot and count its release so Close joins it.
	e.snapMu.Lock()
	retirement := tx.detachSnapshotLocked()
	e.snapRetiring++
	e.snapMu.Unlock()

	// Release the state before the head that backs it.
	if retirement.readTx != nil {
		retirement.readTx.Discard()
	}
	if retirement.readHead != nil {
		retirement.readHead.release()
	}

	// Publish completion for Close.
	e.snapMu.Lock()
	e.snapRetiring--
	e.snapIdle.Broadcast()
	e.snapMu.Unlock()
}

// refreshReadHead adopts the durable head when no local writer owns publication.
// External I/O and retirement run without holding bcast. It takes bcast only
// to adopt a durable head that differs from the published one while no local
// writer owns publication; otherwise the read uses the published head.
func (e *Engine) refreshReadHead(ctx context.Context) error {
	// Skip refresh when no durable head writer is configured.
	if e.writeHeadRefresh == nil {
		return nil
	}

	// A local writer owns publication: read the published head.
	if e.pub.Load() == nil {
		return ErrEngineClosed
	}
	if e.writerOwned.Load() {
		return nil
	}

	// Refresh the durable coordinator head before pinning local state.
	headRef, err := e.writeHeadRefresh(ctx)
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

	// The published head already is the durable head.
	if headRef == nil || headRef.GetRootRef().GetEmpty() {
		return nil
	}
	if p := e.pub.Load(); p != nil && p.ref.EqualsRef(headRef) {
		return nil
	}

	// Revalidate writer ownership after external I/O before adopting the head.
	locked := e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		return ErrEngineClosed
	}
	var retirement engineRetirement
	if e.writeTx == nil && e.writeSession == nil {
		retirement, err = e.applyDurableHeadLocked(ctx, headRef)
	}
	locked.Unlock()
	e.drainRetirement(ctx, retirement)
	return err
}

// refreshWaitHead refreshes the durable head before a wait unless the
// coordinator head watch is healthy. A healthy watch subscribed before its
// first read and adopts every durable head it learns of, including one
// published before the wait, and each adoption wakes the waiters. A point
// read has no later wakeup, so it refreshes through refreshReadHead.
func (e *Engine) refreshWaitHead(ctx context.Context) error {
	// Rely on the head watch while it runs without error.
	locked := e.bcast.Lock()
	healthy := e.headWatchDone != nil && e.headWatchErr == nil
	locked.Unlock()
	if healthy {
		return nil
	}
	return e.refreshReadHead(ctx)
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

// ForkBlockTransaction forks the transaction at the current state. The caller
// of a write fork serializes writers, so the fork's reads report
// block.ErrUnavailable for a block no connected peer holds instead of waiting
// for a peer to connect.
func (e *Engine) ForkBlockTransaction(ctx context.Context, write bool) (*Tx, error) {
	// Trace the fork.
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

	// Buffer nested and root block writes per fork so they drain in one batch
	// at Sync. The buffer records what it writes, so the fork releases blocks
	// its final root does not reach when discarded. Coordinator mode stays
	// durable-on-write: its commit path validates the committed root against
	// raw storage before publication.
	var buffered *block.BufferedStore
	store := block.StoreOps(nil)
	if write && e.writeCoordinator == nil {
		buffered = block.NewBufferedStoreWithSettings(ctx, e.writeBlockStore, &block.BufferedStoreSettings{
			RecordWrites:    true,
			WithoutPeerWait: true,
		})
		store = buffered
	}

	// Build a distinct World state over the retained root.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/build-world-state")
	ws, err := e.buildWorldStateForRoot(taskCtx, !write, e.head.root, store)
	subtask.End()
	if err != nil {
		return nil, err
	}
	ws.ownedStore, ws.releaseOwned = buffered, buffered != nil

	// Hold the base root's pin for the lifetime of a write fork.
	if write {
		ws.readRelease = e.head.pin.hold()
	}

	// Wrap the World state in a transaction.
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/new-tx")
	tx := NewTx(ws)
	subtask.End()
	return tx, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor rejects writes; build through StageWorldState instead.
// Be sure to call Release on the cursor when done.
func (e *Engine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	cursor, err := e.buildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}
	setReadOnlyCursor(ctx, cursor)
	return cursor, nil
}

// buildStorageCursor builds a writable cursor to the world storage.
func (e *Engine) buildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	// Clone the base root under the engine lock.
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

// StageWorldState opens a staging scope over the engine storage.
//
// In deferred durability mode a commit's blocks stay in memory until Sync, so
// the stage flushes them before it releases. Otherwise the sweep could collect
// a build that only a buffered parent references.
func (e *Engine) StageWorldState(ctx context.Context) (world.WorldStage, error) {
	// Reject a closed engine.
	locked := e.bcast.Lock()
	closed := e.closed
	locked.Unlock()
	if closed {
		return nil, ErrEngineClosed
	}

	// Durable-on-write commits reference the build before they return.
	stage := world.NewStorageStage(engineStorage{e: e})
	if !e.deferredDurability() {
		return stage, nil
	}
	return &deferredStage{WorldStage: stage, e: e}, nil
}

// deferredStage flushes the engine's buffered blocks before it releases.
type deferredStage struct {
	world.WorldStage
	e *Engine
}

// Release makes the adopting commits durable, then releases the stage. A
// failed flush keeps the stage until its process owner is reaped; nothing
// retries the flush, so the stage stays held until then.
func (s *deferredStage) Release() {
	if _, err := s.e.writeBlockStore.Sync(context.Background()); err != nil {
		s.e.le.WithError(err).Warn("keeping world stage after failed block flush")
		return
	}
	s.WorldStage.Release()
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor rejects writes and is released after cb returns; build
// through StageWorldState instead.
//
// NOTE: this is the implementation of AccessWorldState for the world/block engine.
func (e *Engine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return e.accessStorage(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
		setReadOnlyCursor(ctx, cursor)
		return cb(cursor)
	})
}

// heldHead is a cursor on a head and the hold that keeps its root pinned. The
// snapshot owning it holds one reference and every object read holds another,
// so a detach releasing the snapshot's reference never releases the cursor
// under a read cloning it.
type heldHead struct {
	// refs counts the holders. Zero means released, and a released head is
	// never held again.
	refs atomic.Int64
	// root is the held cursor; only cloned, never changed.
	root *bucket_lookup.Cursor
	// free drops root and the pin after the last holder.
	free func()
}

// newHeldHead holds root for its first holder until free.
func newHeldHead(root *bucket_lookup.Cursor, free func()) *heldHead {
	h := &heldHead{root: root, free: free}
	h.refs.Store(1)
	return h
}

// tryHold adds a holder unless the head was released.
func (h *heldHead) tryHold() bool {
	// Count up only from a live count, so a released head stays released.
	for {
		n := h.refs.Load()
		if n <= 0 {
			return false
		}
		if h.refs.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// release drops a holder and frees the head after the last one.
func (h *heldHead) release() {
	if h.refs.Add(-1) == 0 {
		h.free()
	}
}

// holdHeadLocked clones the published head and holds its pin until release.
// The pin was taken at publication, so holding it needs no block I/O. Cursor.Clone
// copies the bucket handle without taking a release func, so the hold does not
// establish a cross-bucket lifetime across Engine.Close. The caller must hold
// bcast.
func (e *Engine) holdHeadLocked() (*bucket_lookup.Cursor, func()) {
	held := e.head.root.Clone()
	unpin := e.head.pin.hold()
	return held, func() {
		held.Release()
		unpin()
	}
}

// accessStorage builds a writable bucket lookup cursor with an optional ref,
// holding the published root for the callback without the Engine lock.
// Cursor.Clone copies the bucket handle without taking a release func, so the
// hold does not establish a cross-bucket lifetime across Engine.Close.
func (e *Engine) accessStorage(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	// Hold the published root for the callback.
	head := e.acquireHead()
	if head == nil {
		return ErrEngineClosed
	}
	defer head.release()
	ncs := head.root.Clone()
	defer ncs.Release()
	return e.accessFrom(ctx, ncs, ref, cb)
}

// accessFrom runs cb on root, or on the cursor root follows to ref. The caller
// holds root's pin for the call.
func (e *Engine) accessFrom(
	ctx context.Context,
	root *bucket_lookup.Cursor,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	// Read staged blocks through the root when following a ref.
	if ref != nil && e.stagedStore != nil {
		root.SetTransactionStore(e.stagedStore)
	}

	// Use the root itself when no explicit reference was supplied.
	if ref == nil {
		return cb(root)
	}

	// Follow a supplied root outside the engine lock.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	followed, err := root.FollowRef(subCtx, ref)
	if err != nil {
		return err
	}
	defer followed.Release()

	// Expose the followed cursor only for the callback lifetime.
	return cb(followed)
}

// SetChangelogDisabled turns the World changelog off or on, committing only
// when the setting changes.
//
// Disabling drops the recorded history. Enabling starts the log at the current
// seqno without history.
func (e *Engine) SetChangelogDisabled(ctx context.Context, disable bool) error {
	// Open a write transaction.
	engTx, err := e.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer engTx.Discard()
	if engTx.writeTx == nil {
		return tx.ErrNotWrite
	}

	// Apply the setting and commit only a change.
	unlock, err := engTx.writeTx.rmtx.Lock(ctx, true)
	if err != nil {
		return err
	}
	changed, err := engTx.writeTx.state.SetChangelogDisabled(ctx, disable)
	unlock()
	if err != nil || !changed {
		return err
	}
	return engTx.Commit(ctx)
}

// StepGraphSettings advances the World's graph toward the target settings by
// at most max links in one write transaction, committing only a change. It
// reports whether the graph matches target.
func (e *Engine) StepGraphSettings(ctx context.Context, target *GraphSettings, max int) (bool, error) {
	// Open a write transaction.
	engTx, err := e.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer engTx.Discard()
	if engTx.writeTx == nil {
		return false, tx.ErrNotWrite
	}

	// Apply one step and commit only a change.
	done, changed, err := engTx.StepGraphSettings(ctx, target, max)
	if err != nil || !changed {
		return done, err
	}
	return done, engTx.Commit(ctx)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *Engine) GetSeqno(ctx context.Context) (uint64, error) {
	// Preserve durable freshness without opening a dedicated transaction.
	if err := e.refreshReadHead(ctx); err != nil {
		return 0, err
	}

	// Read the revision of the head published by local commits or durable
	// refresh, without the Engine lock.
	p := e.pub.Load()
	if p == nil {
		return 0, ErrEngineClosed
	}
	return p.seqno, nil
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *Engine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	// Discover durable commits that preceded this wait.
	if err := e.refreshWaitHead(ctx); err != nil {
		return 0, err
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
	// A nil Engine owns nothing to release.
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

	// Cancel the change watch comparison in flight and wake its watchers.
	if w := e.changeWatch; w != nil {
		w.cancel()
		e.stopChangeWatchLocked(w)
	}

	// Detach every Engine-owned resource while publication is closed.
	retirements := make([]engineRetirement, 0, 1)
	retirement := engineRetirement{head: e.head, pub: e.pub.Swap(nil), watched: e.watchedHeads}
	e.head = nil
	e.watchedHeads = nil

	// Detach the write transaction with the head.
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

	// Reject new read snapshots and detach the registered ones.
	e.snapMu.Lock()
	e.snapClosed = true
	for tx := range e.snapshotTxs {
		txRetirement := e.beginRetirementLocked(tx.detachSnapshotLocked())
		if !txRetirement.empty() {
			retirements = append(retirements, txRetirement)
		}
	}
	e.snapMu.Unlock()
	locked.Broadcast()
	locked.Unlock()

	// Drain transaction and coordinator work after unlocking the Engine.
	for _, retirement := range retirements {
		e.drainRetirement(context.Background(), retirement)
	}
	if e.headWatchDone != nil {
		<-e.headWatchDone
	}

	// Join detached work, in-flight commit publication, read opens and change
	// watches before baseRoot release.
	locked = e.bcast.Lock()
	for e.retiring != 0 || e.committing != 0 || e.changeWatches != 0 {
		wait := locked.WaitCh()
		locked.Unlock()
		<-wait
		locked = e.bcast.Lock()
	}

	// Join read opens and snapshot releases, which run without bcast.
	e.snapMu.Lock()
	for e.opening != 0 || e.snapRetiring != 0 {
		e.snapIdle.Wait()
	}
	e.snapMu.Unlock()

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
	// Start the head read-transaction trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/initialize-head-read-tx")
	defer task.End()

	// Keep the initialized transaction when it already matches the head.
	if e.head.readTx != nil &&
		e.head.readTx.state.GetRootRef().EqualsRef(e.head.root.GetRef().GetRootRef()) {
		return nil
	}

	// Publish the restored read transaction together with its root.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/update-read-write-txns/build-world-state")
	world, err := e.buildReadWorldState(taskCtx)
	subtask.End()
	if err != nil {
		return err
	}
	e.head = &engineHead{
		root:   e.head.root,
		pin:    e.head.pin,
		readTx: NewTx(world),
	}
	return nil
}

// buildWriteState builds a write state at root whose blocks go to a buffer
// the transaction owns. The buffer records what it writes, so committing
// drains only the blocks the committed root reaches, even through a parent a
// capacity drain wrote early. Outside a staged session the buffer drains into
// the engine store, and ending the transaction releases the blocks its final
// root does not reach, as a fork does. In a staged session it drains into the
// staged store when the transaction submits, so the next publication carries
// none of a discarded transaction's blocks.
// The caller must hold bcast after construction.
func (e *Engine) buildWriteState(ctx context.Context, root *bucket_lookup.Cursor) (*WorldState, error) {
	// Buffer the writes over the store the engine publishes from.
	inner := e.writeBlockStore
	if e.stagedStore != nil {
		inner = e.stagedStore
	}
	writes := block.NewBufferedStoreWithSettings(ctx, inner, &block.BufferedStoreSettings{RecordWrites: true})

	// Build the state and give it the buffer.
	ws, err := e.buildWorldStateForRoot(ctx, false, root, writes)
	if err != nil {
		return nil, err
	}
	ws.ownedStore, ws.releaseOwned = writes, e.stagedStore == nil
	return ws, nil
}

// buildReadWorldState builds a read-only state at the head root.
// The caller must hold bcast.
func (e *Engine) buildReadWorldState(ctx context.Context) (*WorldState, error) {
	return e.buildWorldStateForRoot(ctx, true, e.head.root, nil)
}

// headRootBlockLocked returns the published head's decoded World root, or nil
// when the head has no read transaction at its root. The block is immutable;
// callers clone it before use. The caller must hold bcast.
func (e *Engine) headRootBlockLocked(ctx context.Context) (*World, error) {
	if e.head == nil || e.head.readTx == nil ||
		!e.head.readTx.state.GetRootRef().EqualsRef(e.head.root.GetRef().GetRootRef()) {
		return nil, nil
	}
	return e.head.readTx.state.GetRoot(ctx)
}

// buildWorldStateForRoot builds state using the engine's stores and lookup policy.
// The caller must hold bcast.
func (e *Engine) buildWorldStateForRoot(
	ctx context.Context,
	readOnly bool,
	root *bucket_lookup.Cursor,
	transactionStore block.StoreOps,
) (*WorldState, error) {
	// Select the store under the existing transaction authority. Both read and
	// write transactions read and write through writeBlockStore so that, in the
	// single-writer deferred path, reads see blocks that are committed in memory
	// but not yet drained to durable storage (read your writes before Sync). It
	// is the bucket store directly in coordinator and self-buffered modes.
	store := transactionStore
	if store == nil {
		store = e.writeBlockStore
	}

	// A reader of the published head shares the head's pin, so opening it
	// does no block I/O under bcast. Other roots take their own pin.
	releaseRoot := func() {}
	atHead := e.head != nil && e.head.root.GetRef().EqualsRef(root.GetRef())
	switch {
	case !readOnly:
	case atHead && e.head.root == root && store == e.writeBlockStore:
		releaseRoot = e.head.pin.hold()
	default:
		var err error
		releaseRoot, err = block.PinRoot(ctx, store, root.GetRef().GetRootRef())
		if err != nil {
			return nil, err
		}
	}

	// The published read head already holds this immutable root.
	var rootBlock *World
	if atHead {
		var err error
		rootBlock, err = e.headRootBlockLocked(ctx)
		if err != nil {
			releaseRoot()
			return nil, err
		}
	}
	return e.newWorldState(ctx, readOnly, root, store, releaseRoot, rootBlock)
}

// newWorldState builds a state at root over store. It reads no Engine fields
// guarded by bcast, so a caller holding root and its pin may build without the
// lock. The state owns releaseRoot, which is called on failure. rootBlock, when
// set, is the decoded World root, cloned so the state owns its mutable fields.
func (e *Engine) newWorldState(
	ctx context.Context,
	readOnly bool,
	root *bucket_lookup.Cursor,
	store block.StoreOps,
	releaseRoot func(),
	rootBlock *World,
) (*WorldState, error) {
	// Trace the build.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state")
	defer task.End()

	// Build the transaction and seed the decoded root.
	btx, bcs := root.BuildTransactionWithStore(nil, store)
	if rootBlock != nil {
		bcs.SetBlock(rootBlock.CloneVT(), false)
	}

	// Build the state. A read-only state writes nothing, so its transaction
	// shares immutable decoded blocks with other readers.
	if readOnly {
		btx.SetReadOnly()
		btx = nil
	}
	ws, err := NewWorldState(ctx, e.le, !readOnly, btx, bcs, store, e, e.lookupOp, e.verbose)
	if err != nil {
		releaseRoot()
		return nil, err
	}

	// The state owns the root pin and resolves same-bucket objects locally.
	ws.readRelease = releaseRoot
	ws.localBucketID = root.GetRefWithOpArgs().GetBucketId()
	return ws, nil
}

// Verify the World engine contract.
var _ world.Engine = (*Engine)(nil)
