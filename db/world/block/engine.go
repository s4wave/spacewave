package world_block

import (
	"context"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// engineHead pairs a published root with the read transaction built from it.
type engineHead struct {
	// root is the cursor on the head root.
	root *bucket_lookup.Cursor
	// readTx reads this root until retirement drains its users.
	readTx *Tx
}

// Engine publishes paired World roots and read snapshots.
// Its head moves only as its Authority decides: a write transaction opens a
// turn, its commit proposes a Candidate, and the engine installs the head the
// Authority accepts once it is durable. Heads another writer committed arrive
// through the Authority's head watch.
// Local read transactions retry when their shared head retires.
type Engine struct {
	// le is the logger.
	le *logrus.Entry
	// lookupOp looks up a world operation.
	lookupOp world.LookupOp
	// verbose enables verbose logging within world state.
	verbose bool
	// authority decides the head, or is nil for an engine whose caller sets
	// the head with SetRootRef and holds its blocks.
	authority world.Authority
	// wmtx ensures only one write transaction is active at a time.
	wmtx csync.Mutex
	// bcast guards Engine head, transaction, lifecycle, and close state.
	bcast broadcast.Broadcast
	// baseRoot is the base root cursor to use.
	// The root cursor is derived with FollowRef from this cursor.
	baseRoot *bucket_lookup.Cursor
	// head pairs the root cursor with its shared read transaction.
	head *engineHead
	// liveMu guards live.
	liveMu sync.Mutex
	// live are the published heads some holder still holds, the current one
	// and each replaced one a reader holds, for ReadRoots.
	live map[*publishedHead]struct{}
	// closing is canceled when Close begins, which ends each commit's wait
	// for its Authority's decision with world.ErrDurabilityUnknown.
	closing context.Context
	// stopClosing cancels closing.
	stopClosing context.CancelCauseFunc
	// headChanged closes when the published head root changes or the Engine
	// closes. Revision waiters wait on it instead of bcast, which also wakes
	// on every transaction retirement. Guarded by bcast.
	headChanged chan struct{}
	// writeTx is the current write transaction, canceled if its head changes.
	writeTx *EngineTx
	// writeTxRel releases wmtx after the detached write transaction drains.
	writeTxRel func()
	// pub is the published head read transactions open at. It is swapped
	// under bcast and loaded without it, so a read open never waits on a
	// writer holding bcast. Nil after Close.
	pub atomic.Pointer[publishedHead]
	// snapMu guards read snapshot registration. It is never held across I/O,
	// and bcast may be taken before it but never while holding it.
	snapMu sync.Mutex
	// snapIdle wakes Close when opening or snapRetiring reaches zero.
	snapIdle *sync.Cond
	// snapChanged is closed and replaced when a read open ends or a snapshot
	// finishes releasing, for WaitIdle. Guarded by snapMu.
	snapChanged chan struct{}
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
	// committing counts write commits that own Authority work outside bcast.
	committing int
	// durableSeqno is the seqno of the durable head: every commit at or below
	// it is durable. A head publishes only once the Authority reports it
	// durable, so it follows the published head, except in deferred mode,
	// where only Sync advances it. Guarded by bcast.
	durableSeqno uint64
	// writeBlockStore is the long-lived block store backing transactions. It
	// is the bucket store, or, in deferred-durability mode, a BufferedStore
	// wrapping it, so commits accumulate in memory until Sync. Stable for the
	// engine lifetime because the world cursor never switches buckets.
	writeBlockStore block.StoreOps
	// deferDurability enables deferred durability for an engine without an
	// Authority: commits stay in memory until Sync.
	deferDurability bool
	// headWatchRelease stops the Authority's head watch, or is nil.
	headWatchRelease func()
	// readerPinDelay is how long a replaced head a reader holds waits before
	// the engine pins it.
	readerPinDelay time.Duration
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
	// retired reports that Retire closed the engine for a successor, so the
	// engine and the transactions it ended report errEngineRetired.
	retired atomic.Bool
	// direct is the head the engine last published itself, from a commit or
	// a turn's base, until the Authority's head watch reports it. Until then
	// a watched head is installed only when it is newer than direct, so a
	// late report of an older head never rolls the published head back.
	// Nil while the published head is the watch's. Guarded by bcast.
	direct *world.Head
	// admitted is the root of the last candidate this engine admitted, with
	// its decoded World root, so a turn whose base is that candidate builds
	// on it without reading the root block. Guarded by bcast.
	admitted *admittedHead
	// recordOps makes write transactions record their World operations, so
	// a commit's Candidate carries them as its Operation.
	recordOps bool
}

// admittedHead is a candidate root an Authority admitted and its decoded World
// root.
type admittedHead struct {
	// root is the candidate's World root.
	root *block.BlockRef
	// world is the decoded World root block.
	world *World
}

// OperationAuthor returns no signer for a standalone block World.
func (e *Engine) OperationAuthor(context.Context) (peer.ID, string, error) {
	return "", "", nil
}

// EngineOption configures optional Engine integrations.
type EngineOption func(*Engine)

// WithDeferredDurability keeps the commits of an engine without an Authority
// in memory until Sync. An engine with an Authority ignores it: its commits
// are durable when the Authority reports them so.
func WithDeferredDurability() EngineOption {
	return func(e *Engine) {
		e.deferDurability = true
	}
}

// WithRecordedOperations makes write transactions record the World
// operations they apply, so a commit proposes them to the Authority as the
// Candidate's Operation: the encoded world_block_tx batch, with the engine's
// own bucket ID cleared from its object roots. An Authority that replays
// operations, such as a Space's, needs it; the local Authority does not.
func WithRecordedOperations() EngineOption {
	return func(e *Engine) {
		e.recordOps = true
	}
}

// WithReaderPinDelay sets how long a replaced head a reader still holds waits
// before the engine pins its root, one minute by default. Zero pins it before
// the commit that replaced it returns, for a Volume whose grace period is
// shorter than any reader.
func WithReaderPinDelay(delay time.Duration) EngineOption {
	return func(e *Engine) {
		e.readerPinDelay = delay
	}
}

// NewEngine constructs a new world engine at root.
//
// authority decides the head. root must be at the Authority's head.
// authority can be nil for an engine whose caller sets the head with
// SetRootRef and holds the blocks its commits store.
func NewEngine(
	ctx context.Context,
	le *logrus.Entry,
	root *bucket_lookup.Cursor,
	lookupOp world.LookupOp,
	authority world.Authority,
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
		authority:      authority,
		head:           &engineHead{root: root.Clone()},
		headChanged:    make(chan struct{}),
		verbose:        verbose,
		snapshotTxs:    make(map[*EngineTx]struct{}),
		snapChanged:    make(chan struct{}),
		live:           make(map[*publishedHead]struct{}),
		watchedHeads:   make([]*watchedHead, 0, changeWatchHeads),
		readerPinDelay: defaultReaderPinDelay,
	}
	e.snapIdle = sync.NewCond(&e.snapMu)
	e.closing, e.stopClosing = context.WithCancelCause(context.Background())
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}

	// Without an Authority, deferred durability keeps commits in one
	// long-lived BufferedStore until Sync. A self-buffered store owns its own
	// intake and Sync fence.
	rawWriteStore := e.head.root.GetBucket()
	e.writeBlockStore = rawWriteStore
	if e.deferredDurability() && rawWriteStore.GetSupportedFeatures()&block.StoreFeatureSelfBuffered == 0 {
		e.writeBlockStore = block.NewBufferedStore(ctx, rawWriteStore)
	}

	// Publish a complete read head before returning the engine.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new/initialize-head-read-tx")
	err := e.initializeHeadReadTx(taskCtx)
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
	e.publishHeadLocked(ctx, e.durableSeqno)
	e.direct = &world.Head{Root: e.head.root.GetRef().GetRootRef().Clone(), Seqno: e.durableSeqno, Durable: true}

	// Follow the heads other writers commit for the engine's lifetime.
	// Constructor cancellation does not end it.
	if authority != nil {
		release, err := authority.WatchHead(context.WithoutCancel(ctx), e.followHead)
		if err != nil {
			_ = e.Close()
			return nil, err
		}
		locked := e.bcast.Lock()
		e.headWatchRelease = release
		locked.Unlock()
	}
	return e, nil
}

// deferredDurability reports whether commits stay in memory until Sync: an
// engine without an Authority that asked for it.
func (e *Engine) deferredDurability() bool {
	return e.deferDurability && e.authority == nil
}

// GetRootRef gets the current root cursor reference.
func (e *Engine) GetRootRef() *bucket.ObjectRef {
	// Clone the published head ref without the engine lock.
	if p := e.pub.Load(); p != nil {
		return p.ref.Clone()
	}
	return nil
}

// Sync fences durable storage.
//
// A commit through an Authority is durable when it returns, so this runs the
// store's fence only. In deferred-durability mode it makes every commit so
// far durable and advances the durable seqno to the published head.
func (e *Engine) Sync(ctx context.Context) (bool, error) {
	// Hold publication stable across the durability fence.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/sync")
	defer task.End()
	locked := e.bcast.Lock()
	defer locked.Unlock()
	if e.closed {
		return false, e.closedErr()
	}
	return e.syncHeadLocked(ctx)
}

// SyncThrough returns once every commit with seqno <= seqno is durable and
// covered by the durable head, and returns the durable seqno, >= seqno.
//
// It waits only for the published head to reach seqno, and joins no later
// commit. An engine with an Authority publishes a head only once it is
// durable, so a head at seqno needs no fence. In deferred-durability mode a
// head not yet covered by the durable seqno takes the same fence as Sync: the
// shared BufferedStore flushes every pending block, not only those reachable
// from seqno, and the durable seqno advances to the published head, which can
// be newer than seqno.
func (e *Engine) SyncThrough(ctx context.Context, seqno uint64) (uint64, error) {
	// Trace the fence as one task.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/sync-through")
	defer task.End()

	// Wait for the head to publish seqno, unless the durable head covers it.
	for {
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return 0, e.closedErr()
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

// syncHeadLocked runs the store's durability fence and records that the
// published head is durable. The caller must hold bcast.
func (e *Engine) syncHeadLocked(ctx context.Context) (bool, error) {
	fenced, err := e.writeBlockStore.Sync(ctx)
	if err != nil {
		return false, err
	}
	e.durableSeqno = max(e.durableSeqno, e.headSeqnoLocked())
	return fenced, nil
}

// SetRootRef installs ref as the head of an engine without an Authority.
// Re-creates the internal read transaction with the updated state.
// Cancels any ongoing write tx (to be re-created against new state).
// Can return an error to indicate validation failure.
// An engine with an Authority refuses it: the Authority owns its head.
func (e *Engine) SetRootRef(ctx context.Context, ref *bucket.ObjectRef) error {
	// Refuse a head the Authority did not decide, and decode its World root
	// before taking the Engine lock.
	if e.authority != nil {
		return ErrAuthorityHead
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	rootBlock, err := e.decodeRoot(ctx, ref)
	if err != nil {
		return err
	}

	// Install the head under the Engine lock.
	locked := e.bcast.Lock()
	if e.closed {
		locked.Unlock()
		return e.closedErr()
	}
	retirement, err := e.installRootRefLocked(ctx, ref, true, rootBlock)
	locked.Unlock()

	// Retired readers may reenter the engine during cleanup.
	e.drainRetirement(ctx, retirement)
	return err
}

// followHead installs a head the Authority's watch reports. The watch reports
// heads in the order the Authority decides them, so a report is installed as
// it comes, unless the engine published a head itself since the watch's last
// report: a commit's head or a turn's base. Until the watch reports that head,
// a report is installed only when its seqno is past it, or at its seqno with
// another root that is still the Authority's head (one indexed root read), so
// a late report of an older head never rolls the head back. A report while a
// local writer holds a turn is installed without ending the writer: the
// writer's candidate goes to the Authority on its own base, and the Authority
// decides it against the head it holds.
func (e *Engine) followHead(head world.Head) {
	// Skip an empty head and the published one, which ends a direct head the
	// watch now reports.
	ctx := context.Background()
	if head.Root.GetEmpty() {
		return
	}
	locked := e.bcast.Lock()
	p := e.pub.Load()
	direct := e.direct
	if direct != nil && head.Root.EqualsRef(direct.Root) {
		e.direct = nil
		direct = nil
	}
	if e.closed || p == nil || head.Root.EqualsRef(p.ref.GetRootRef()) {
		locked.Unlock()
		return
	}
	locked.Unlock()

	// Order the report against a head the engine published itself.
	if direct != nil {
		switch {
		case head.Seqno > direct.Seqno:
		case head.Seqno == direct.Seqno:
			current, err := e.authority.Head(ctx)
			if err != nil || !current.Root.EqualsRef(head.Root) {
				return
			}
		default:
			return
		}
	}

	// Decode the head's World root before taking the Engine lock.
	ref := p.ref.Clone()
	ref.RootRef = head.Root.Clone()
	rootBlock, err := e.decodeRoot(ctx, ref)
	if err != nil {
		if !errors.Is(err, ErrEngineClosed) {
			e.le.WithError(err).Warn("unable to read the World head the authority reported")
		}
		return
	}

	// Hold the engine unless the published head moved meanwhile.
	locked = e.bcast.Lock()
	if e.closed || e.pub.Load() != p {
		locked.Unlock()
		return
	}

	// Install the head, keeping a local writer, then drain what it replaced.
	retirement, err := e.installRootRefLocked(ctx, ref, false, rootBlock)
	if err == nil {
		e.direct = nil
	}
	locked.Unlock()
	e.drainRetirement(ctx, retirement)
	if err != nil {
		e.le.WithError(err).Warn("unable to install the World head the authority reported")
	}
}

// decodeRoot decodes the World root ref names without the Engine lock, so an
// install under the lock decodes nothing. Close joins it as a read open.
func (e *Engine) decodeRoot(ctx context.Context, ref *bucket.ObjectRef) (*World, error) {
	// Count the decode so Close joins it, unless the Engine has closed.
	e.snapMu.Lock()
	if e.snapClosed {
		e.snapMu.Unlock()
		return nil, ErrEngineClosed
	}
	e.opening++
	e.snapMu.Unlock()
	defer func() {
		e.snapMu.Lock()
		e.opening--
		e.signalSnapLocked()
		e.snapMu.Unlock()
	}()

	// Follow the ref and decode its root.
	root, err := e.baseRoot.FollowRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer root.Release()
	_, bcs := root.BuildTransactionWithStore(nil, e.writeBlockStore)
	return UnmarshalWorld(ctx, bcs)
}

// installRootRefLocked can preserve the active provisional successor. Durable
// completion of N must never discard the already preparing N+1 transaction.
func (e *Engine) installRootRefLocked(ctx context.Context, ref *bucket.ObjectRef, invalidateWriter bool, rootBlock *World) (engineRetirement, error) {
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

	// Construct the replacement read transaction before changing the head,
	// from the decoded root a committing writer already holds, when it does.
	// The head's own reader takes no pin: the Authority's root holds the head.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/build-world-state")
	nextWorld, err := e.newWorldState(taskCtx, true, nextRoot, e.writeBlockStore, func() {}, rootBlock)
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

	// The Authority's root holds the head while it is current, so publishing
	// it takes no pin; a replaced head held by a reader pins later.
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
		retirement.turn = writeRetirement.turn
		retirement.writeTxRel = writeRetirement.writeTxRel
	}
	e.head = &engineHead{
		root:   nextRoot,
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
// retires after unlocking. The caller must hold bcast.
func (e *Engine) publishHeadLocked(ctx context.Context, seqno uint64) *publishedHead {
	// Seed readers with the decoded root when the head has one; a reader
	// without it decodes the root itself.
	rootBlock, err := e.headRootBlockLocked(ctx)
	if err != nil {
		rootBlock = nil
	}
	p := newPublishedHead(e.head.root, rootBlock, seqno)
	e.liveMu.Lock()
	e.live[p] = struct{}{}
	e.liveMu.Unlock()
	p.gone = func() {
		e.liveMu.Lock()
		delete(e.live, p)
		e.liveMu.Unlock()
	}
	return e.pub.Swap(p)
}

// acquireHead holds the published head without the Engine lock, or returns
// nil after Close. A retired head was already replaced, so the retry loads a
// newer one. A head replaced while the hold was taken is released and the
// retry takes the newer one, so every holder of a replaced head was counted
// before the replacement retired it and armed its reader pin.
func (e *Engine) acquireHead() *publishedHead {
	for {
		// Hold the published head.
		p := e.pub.Load()
		if p == nil {
			return nil
		}
		if !p.tryHold() {
			continue
		}

		// Keep it only while it is still the published head.
		if e.pub.Load() == p {
			return p
		}
		p.release()
	}
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

// drainRetirement waits for detached transaction users before releasing their
// cursor, Authority turn, and writer slot.
func (e *Engine) drainRetirement(ctx context.Context, retirement engineRetirement) {
	// Return when the retirement has nothing to drain.
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
	// Readers still holding a replaced published head keep its root, and pin
	// it if they hold it long; a closing engine pins nothing.
	if retirement.pub != nil {
		if retirement.closing {
			retirement.pub.release()
		} else {
			retirement.pub.retire(e.le, e.writeBlockStore, e.readerPinDelay)
		}
	}
	if retirement.readHead != nil {
		retirement.readHead.release()
	}
	if retirement.head != nil && retirement.head.root != nil {
		retirement.head.root.Release()
	}
	for _, h := range retirement.watched {
		h.root.Release()
	}
	if retirement.base != nil {
		retirement.base.Release()
	}

	// End the writer's Authority turn, then its local writer slot.
	if retirement.turn != nil {
		retirement.turn.Release()
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

// signalSnapLocked wakes Close and WaitIdle after a read open ends or a
// snapshot finishes releasing. The caller must hold snapMu.
func (e *Engine) signalSnapLocked() {
	e.snapIdle.Broadcast()
	close(e.snapChanged)
	e.snapChanged = make(chan struct{})
}

// WaitIdle returns once no transaction is open on the engine: no read
// snapshot opening or held, no write transaction, and no commit or release
// under way. It does not stop new transactions, so a caller that retires the
// engine stops handing it out first, and then lets the transactions already
// open finish before Close discards them.
func (e *Engine) WaitIdle(ctx context.Context) error {
	for {
		// Read the snapshot count and the wait for its next change.
		e.snapMu.Lock()
		snaps := len(e.snapshotTxs) + e.opening + e.snapRetiring
		snapWait := e.snapChanged
		e.snapMu.Unlock()

		// Read the writer state and the wait for its next change.
		locked := e.bcast.Lock()
		busy := e.writeTx != nil || e.committing != 0 || e.retiring != 0
		wait := locked.WaitCh()
		locked.Unlock()
		if snaps == 0 && !busy {
			return nil
		}

		// Wait for either to change.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-wait:
		case <-snapWait:
		}
	}
}

// finishCommit publishes completion after coordinator commit cleanup finishes.
func (e *Engine) finishCommit() {
	locked := e.bcast.Lock()
	e.committing--
	locked.Broadcast()
	locked.Unlock()
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
// An engine that records operations wraps a write transaction in a recorder
// whose commit proposes the operations it applied.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	etx, err := e.NewBlockEngineTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	if !write || !e.recordOps {
		return etx, nil
	}
	return newRecordedTx(ctx, etx)
}

// NewBlockEngineTransaction returns the world-block specific EngineTx type.
//
// A write transaction holds the engine's writer slot and its Authority's turn
// until Commit or Discard, and builds on the turn's base.
func (e *Engine) NewBlockEngineTransaction(ctx context.Context, write bool) (*EngineTx, error) {
	// Start the transaction trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction")
	defer task.End()

	// Read-only transactions retain one immutable World revision until Discard.
	if !write {
		return e.openReadTx(ctx)
	}

	// Serialize writers until Commit or Discard drains the prior transaction.
	relLock, err := e.wmtx.Lock(ctx)
	if err != nil {
		return nil, err
	}

	// Take the Authority's turn and the head it builds on.
	turn, err := e.beginTurn(ctx)
	if err != nil {
		relLock()
		return nil, err
	}
	base := turn.Base()

	// Trace building the write state, and decode the World root of a base
	// readers do not see yet before taking the engine lock.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction/build-world-state")
	defer subtask.End()
	var baseBlock *World
	if p := e.pub.Load(); p != nil && !base.Root.GetEmpty() && !base.Root.EqualsRef(p.ref.GetRootRef()) {
		ref := p.ref.Clone()
		ref.RootRef = base.Root.Clone()
		if baseBlock, err = e.decodeRoot(taskCtx, ref); err != nil {
			turn.Release()
			relLock()
			return nil, err
		}
	}
	locked := e.bcast.Lock()

	// A failure releases the base's replacement, the turn and the writer
	// slot.
	fail := func(retirement engineRetirement, err error) (*EngineTx, error) {
		// Release what the open took, newest first.
		locked.Unlock()
		e.drainRetirement(ctx, retirement)
		turn.Release()
		relLock()

		// A base remapped under the writer is a superseded one.
		if isCoordinatedWriteSnapshotError(err) {
			err = stderrors.Join(errors.Wrap(world.ErrSuperseded, "build world state"), err)
		}
		return nil, err
	}
	if e.closed {
		return fail(engineRetirement{}, e.closedErr())
	}
	var retirement engineRetirement
	root, rootBlock := e.head.root, (*World)(nil)
	var private *bucket_lookup.Cursor
	if !base.Root.GetEmpty() && !base.Root.EqualsRef(e.head.root.GetRef().GetRootRef()) {
		ref := e.head.root.GetRef().Clone()
		ref.RootRef = base.Root.Clone()
		switch {
		case base.Durable && base.Seqno >= e.headSeqnoLocked():
			// Publish a newer durable base another writer committed.
			if retirement, err = e.installRootRefLocked(taskCtx, ref, true, baseBlock); err != nil {
				return fail(retirement, err)
			}
			e.direct = &world.Head{Root: base.Root.Clone(), Seqno: base.Seqno, Durable: true}
			root = e.head.root
		default:
			// Build on a base readers do not see yet, such as a candidate
			// admitted and not yet durable, without publishing it.
			if private, err = e.baseRoot.FollowRef(taskCtx, ref); err != nil {
				return fail(retirement, err)
			}
			root, rootBlock = private, baseBlock
			if a := e.admitted; a != nil && a.root.EqualsRef(base.Root) {
				rootBlock = a.world
			}
		}
	}

	// Build the write state on the base.
	baseHeadRef := root.GetRef().Clone()
	world, err := e.buildWriteState(taskCtx, root, rootBlock)
	if err != nil {
		if private != nil {
			private.Release()
		}
		return fail(retirement, err)
	}

	// Register the writer only once it carries its base and turn.
	engTx := newEngineTx(e, NewTx(world))
	engTx.baseHeadRef = baseHeadRef
	engTx.privateBase = private
	engTx.turn = turn
	e.writeTx = engTx
	e.writeTxRel = relLock
	locked.Unlock()

	// Drain the head the base replaced and return the write transaction.
	e.drainRetirement(ctx, retirement)
	return engTx, nil
}

// beginTurn opens the Authority's write turn, or the caller's turn of an
// engine without one.
func (e *Engine) beginTurn(ctx context.Context) (world.Turn, error) {
	if e.authority == nil {
		return &callerTurn{e: e}, nil
	}
	return e.authority.Begin(ctx)
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
		return nil, e.closedErr()
	}
	e.opening++
	e.snapMu.Unlock()

	// Hold the published head for the snapshot's object reads and for its
	// state, and build the state from it.
	var engTx *EngineTx
	var world *WorldState
	err := e.closedErr()
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
	e.signalSnapLocked()
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
		err = e.closedErr()
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
	e.signalSnapLocked()
	e.snapMu.Unlock()
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
		return nil, e.closedErr()
	}

	// Buffer nested and root block writes per fork so they drain in one batch
	// at Sync. The buffer records what it writes, so the fork releases blocks
	// its final root does not reach when discarded. Coordinator mode stays
	// durable-on-write: its commit path validates the committed root against
	// raw storage before publication.
	var buffered *block.BufferedStore
	store := block.StoreOps(nil)
	if write {
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

	// Hold the published head for the lifetime of the fork, so a fork that
	// outlives the head pins it.
	ws.readRelease = e.pub.Load().hold()

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
		return nil, e.closedErr()
	}
	ncs := e.baseRoot.Clone()
	ncs.SetRootRef(nil)
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
		return nil, e.closedErr()
	}

	// Pin the head the stage builds over, so a long session whose base main
	// moves off keeps the base pages its puts name, until the stage releases.
	unpin, err := block.PinRoot(ctx, e.writeBlockStore, e.GetRootRef().GetRootRef())
	if err != nil {
		return nil, err
	}
	var stage world.WorldStage = &pinnedStage{WorldStage: world.NewStorageStage(engineStorage{e: e}), unpin: unpin}

	// Durable-on-write commits reference the build before they return.
	if !e.deferredDurability() {
		return stage, nil
	}
	return &deferredStage{WorldStage: stage, e: e}, nil
}

// pinnedStage releases the pin on its base head after the stage.
type pinnedStage struct {
	world.WorldStage
	// unpin releases the base head's pin.
	unpin func()
}

// Release releases the stage, then its base head's pin.
func (s *pinnedStage) Release() {
	s.WorldStage.Release()
	s.unpin()
}

// deferredStage flushes the engine's buffered blocks before it releases.
type deferredStage struct {
	world.WorldStage
	e *Engine
}

// Release makes the adopting commits durable, then releases the stage. A
// failed flush keeps the stage until its process owner is reaped.
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

// holdHeadLocked clones the published head and holds it until release, so a
// hold that outlives the head pins it. Holding takes no block I/O. Cursor.Clone
// copies the bucket handle without taking a release func, so the hold does not
// establish a cross-bucket lifetime across Engine.Close. The caller must hold
// bcast.
func (e *Engine) holdHeadLocked() (*bucket_lookup.Cursor, func()) {
	held := e.head.root.Clone()
	release := e.pub.Load().hold()
	return held, func() {
		held.Release()
		release()
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
		return e.closedErr()
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
	// Read the revision of the published head without the Engine lock.
	p := e.pub.Load()
	if p == nil {
		return 0, e.closedErr()
	}
	return p.seqno, nil
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *Engine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	// Capture the head and its next publication event under the same lock.
	for {
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return 0, e.closedErr()
		}
		if err := e.initializeHeadReadTx(ctx); err != nil {
			locked.Unlock()
			return 0, err
		}
		readTx := e.head.readTx
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

		// Return a satisfied revision or wait for the next head.
		if seqno >= value {
			return seqno, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-wait:
		}
	}
}

// closedErr returns the error a closed engine reports: errEngineRetired once
// Retire closed it, and ErrEngineClosed otherwise.
func (e *Engine) closedErr() error {
	if e.retired.Load() {
		return errEngineRetired
	}
	return ErrEngineClosed
}

// Retire closes the engine for a successor that serves the same World. Unlike
// Close, the engine and every transaction it ends report
// world.ErrEngineRetired, a supersession, so each caller runs its change again
// on the successor at once instead of the engine waiting for them to finish.
// A commit already admitted still ends with world.ErrDurabilityUnknown.
func (e *Engine) Retire() error {
	e.retired.Store(true)
	return e.Close()
}

// Close releases root cursors and active transaction state owned by the engine.
func (e *Engine) Close() error {
	// A nil Engine owns nothing to release.
	if e == nil {
		return nil
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
	e.stopClosing(world.ErrDurabilityUnknown)
	e.signalHeadLocked()

	// Cancel the change watch comparison in flight and wake its watchers.
	if w := e.changeWatch; w != nil {
		w.cancel()
		e.stopChangeWatchLocked(w)
	}

	// Detach every Engine-owned resource while publication is closed.
	retirements := make([]engineRetirement, 0, 1)
	retirement := engineRetirement{head: e.head, pub: e.pub.Swap(nil), watched: e.watchedHeads, closing: true}
	headWatchRelease := e.headWatchRelease
	e.headWatchRelease = nil
	e.head = nil
	e.watchedHeads = nil

	// Detach the write transaction with the head, unless its commit is under
	// way: the commit ends it after its swap, releasing its stage then, and
	// Close joins the commit below.
	if e.writeTx != nil && !e.writeTx.committing {
		writeRetirement := e.writeTx.detachLocked()
		retirement.readTx = writeRetirement.readTx
		retirement.writeTx = writeRetirement.writeTx
		retirement.turn = writeRetirement.turn
		retirement.base = writeRetirement.base
		retirement.writeTxRel = writeRetirement.writeTxRel
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

	// Stop the Authority's head watch, which may be installing a head under
	// bcast, then drain transactions after unlocking the Engine.
	if headWatchRelease != nil {
		headWatchRelease()
	}
	for _, retirement := range retirements {
		if e.retired.Load() {
			retirement.end(errEngineRetired)
		}
		e.drainRetirement(context.Background(), retirement)
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
		readTx: NewTx(world),
	}
	return nil
}

// buildWriteState builds a write state at root whose blocks go to a buffer
// the transaction owns. The buffer records what it writes, so committing
// hands the Authority only the blocks the committed root reaches, even through
// a parent a capacity drain wrote early. Ending the transaction releases the
// blocks its final root does not reach, as a fork does.
//
// The buffer drains under a stage the transaction holds until it ends, after
// its commit swapped the head in or it was abandoned. A transaction too large
// for the buffer drains early, and one that runs past the collector's grace
// period, such as a large import or a machine asleep mid-commit, would
// otherwise leave its early drains unheld. A stage that writes nothing costs
// no Volume transaction.
// The caller must hold bcast after construction.
// rootBlock, when set, is the decoded World root at root, which a writer
// building on an admitted candidate's root takes from the engine.
func (e *Engine) buildWriteState(ctx context.Context, root *bucket_lookup.Cursor, rootBlock *World) (*WorldState, error) {
	// Open the stage the early drains write under.
	staged, releaseStage, err := block.OpenStage(ctx, e.writeBlockStore)
	if err != nil {
		return nil, err
	}

	// Buffer the writes over the stage.
	writes := block.NewBufferedStoreWithSettings(ctx, staged, &block.BufferedStoreSettings{RecordWrites: true})

	// Build the state and give it the buffer and the stage.
	var ws *WorldState
	if rootBlock != nil {
		ws, err = e.newWorldState(ctx, false, root, writes, func() {}, rootBlock)
	} else {
		ws, err = e.buildWorldStateForRoot(ctx, false, root, writes)
	}
	if err != nil {
		releaseStage()
		return nil, err
	}
	ws.ownedStore, ws.releaseOwned = writes, true
	ws.readRelease = releaseStage
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
	// write transactions read and write through writeBlockStore so that, in
	// deferred mode, reads see blocks that are committed in memory but not yet
	// drained to durable storage (read your writes before Sync).
	store := transactionStore
	if store == nil {
		store = e.writeBlockStore
	}

	// The Authority's root holds the published head and a writer's base, so
	// reading them takes no pin and opening does no block I/O under bcast.
	// Another root takes its own pin.
	releaseRoot := func() {}
	atHead := e.head != nil && e.head.root.GetRef().EqualsRef(root.GetRef())
	switch {
	case !readOnly:
	case atHead && e.head.root == root && store == e.writeBlockStore:
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
