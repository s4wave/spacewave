package space_world

import (
	"bytes"
	"context"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space/world/cursorcodec"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// ReplayIdentityEnv selects the legacy matcher when set to "off" at open.
const ReplayIdentityEnv = "SPACEWAVE_WORLD_REPLAY_IDENTITY"

// OrderedHoldEnv selects durable admission holds when set to "off" at open.
// Otherwise BlockWriteOrderer lets the queued state write flush its hold too.
const OrderedHoldEnv = "SPACEWAVE_WORLD_ORDERED_HOLD"

// RefusalHandoffEnv selects local-ID-only legacy refusals when "off" at startup.
const RefusalHandoffEnv = "SPACEWAVE_WORLD_REFUSAL_HANDOFF"

// refusalHandoffOff selects the previous refusal path at startup.
var refusalHandoffOff = os.Getenv(RefusalHandoffEnv) == "off"

// Adapter implements the Space's replay body and World authority.
//
// Writes pipeline from admitted candidates; replay decides their outcomes.
// The operation log and checkpoint provide durability while intermediate pages
// stay buffered. Payload holds let successors replay after their author leaves.
//
// Invariants:
//   - The adopted checkpoint is always held by replay-base or checkpoint.
//   - A saved cursor's World is held by replay-cursor before its record moves.
//   - Each candidate starts from the last pending candidate or installed head.
//   - An installed head keeps every previously applied operation until replay
//     places or refuses it again.
//   - Every head watcher sees an accepted candidate's head before its decision.
//   - Root moves hold all gatherLive Worlds before releasing old holders.
type Adapter struct {
	// le is the logger.
	le *logrus.Entry
	// opened gates body calls until Execute claims the Space and loads its roots.
	opened chan struct{}
	// so is the Space's SharedObject.
	so sobject.SharedObject
	// opts are the adapter's options.
	opts AdapterOptions
	// lookupOp resolves World operations: the built-in types, then the bus.
	lookupOp world.LookupOp
	// payloadVersionsMtx guards the driver and admission version cache.
	payloadVersionsMtx sync.Mutex
	// payloadVersions caches OpTypes.payloadVersion by operation type.
	payloadVersions map[string]uint32
	// bucketID is the SharedObject block store's bucket.
	bucketID string
	// cursors builds cursors with the shared World decode cache.
	cursors cursorcodec.Factory
	// pages buffers World pages for both the serving and replay engines.
	pages *block.PendingStore
	// queue queues admitted operations, in admission order.
	queue *opQueue
	// backfill keeps a local copy of the World while this device chose it.
	backfill *spaceBackfill
	// replay holds positions above the checkpoint, owned by the driver goroutine.
	replay *spaceReplay
	// missing blocks head publication until replay replaces reordered operations.
	// The driver goroutine owns it.
	missing map[string]struct{}
	// turnMtx serializes proposals from Begin to Release.
	turnMtx csync.Mutex
	// rootMtx serializes root swaps after lockRoots flushes their pages.
	rootMtx csync.Mutex
	// held maps root names to refs under rootMtx.
	held map[string]*block.BlockRef
	// payloads are the payload roots the replay payloads root holds.
	payloads rootSet
	// stateHold tracks the last order Hold saw, owned by the driver goroutine.
	stateHold *payloadState
	// resetPlanned lets tests park a payload reset before it takes rootMtx.
	resetPlanned func()
	// retained is the RootRetained root's state machine.
	retained retainedHold
	// cursorWorlds, pendingWorlds and checkpointWorlds hold World histories
	// until checkpoint coverage releases them.
	cursorWorlds, pendingWorlds, checkpointWorlds rootSet
	// imported holds HoldImport roots until replay-base or payloads holds them.
	imported rootSet
	// watchMtx serializes the head watchers' callbacks.
	watchMtx sync.Mutex
	// reported is the last head reported, guarded by watchMtx.
	reported world.Head

	// bcast guards fields below unless another owner is stated.
	bcast broadcast.Broadcast
	// head is the installed head.
	head world.Head
	// headState is the World state of head.
	headState *InnerState
	// writer is the last pending candidate head, or head.
	writer world.Head
	// pending holds undecided admissions in admission order.
	pending []*spaceAdmission
	// fences maps applied fence hashes to answers.
	fences map[string]*spaceFence
	// legacyReplayMatcher selects the pre-receipt replay and queue paths.
	legacyReplayMatcher bool
	// expectWrites enables ExpectedWrite when signer receipts are available.
	expectWrites bool
	// orderedHolds selects write ordering for holdAdmitted; see OrderedHoldEnv.
	orderedHolds bool
	// unmatched holds early refusals only in legacy mode.
	unmatched map[string]spacePosition
	// cover records installed checkpoint coverage, nil before the first install.
	cover *checkpointCover
	// watchers receive each installed head.
	watchers []*headWatch
	// installed records whether the first checkpoint was installed.
	installed bool
	// checkpoint and checkpointSeqno are the checkpoint backfill copies.
	checkpoint      *InnerState
	checkpointSeqno uint64
	// headHeight and headPositions locate head in checkpoint and replay order.
	headHeight    uint64
	headPositions int
	// acceptHeight records the last fully published checkpoint height.
	// Covered admissions wait until retained roots and head watchers reach it.
	acceptHeight uint64
	// replayState drives gateWrite, starting in Opening.
	replayState *sobject.SOReplayState
	// handlerWait belongs to the body driver and waits for a replay handler.
	handlerWait *handlerWait
	// engine serves from publication until retirement.
	engine *Engine
	// storing keeps background roots live by token until their stores return.
	// Execute drains these holds before releasing the Space.
	storing map[uint64][]*block.BlockRef
	// storingNext allocates background store tokens.
	storingNext uint64
	// stopped refuses new background stores after Execute ends.
	stopped bool
	// reloading fences later writes until retirement after an unknown outcome.
	reloading bool
	// settles tracks this device's unknown writes across adapters.
	settles *settleSet
	// settleSeq bounds which earlier writes the opening order can settle.
	// It is fixed before opened closes.
	settleSeq uint64
	// runCtx ends every background store when Execute's run ends.
	runCtx context.Context
	// cursorSaving reports that a cursor save runs in the background.
	cursorSaving atomic.Bool
	// trimMtx runs the background trims one at a time.
	trimMtx csync.Mutex
	// trimSeq orders gathered plans so an older trim cannot move the base back.
	trimSeq atomic.Uint64
	// readers keeps engine snapshots live through retirement until engine close.
	readers worldReaders
	// holders counts the AcquireEngine holds on engine.
	holders int
	// ownership caches ownershipHead, guarded by bcast.
	ownership     *sobject.SOOwnership
	ownershipHead []byte
}

// headWatch is one WatchHead registration.
type headWatch struct {
	// fn receives each installed head.
	fn func(world.Head)
}

// NewAdapter returns the adapter of the Space World of so. Execute serves it.
func NewAdapter(le *logrus.Entry, so sobject.SharedObject, opts AdapterOptions) (*Adapter, error) {
	// Require a transform factory.
	if opts.StepFactorySet == nil {
		return nil, errors.New("space world adapter requires a step factory set")
	}
	if opts.MaxPendingBytes == 0 && !opts.transient {
		opts.MaxPendingBytes = defaultMaxPendingBytes
	}
	settles := opts.settles
	if settles == nil {
		settles = newSettleSet()
	}

	// Share page buffering and decoding between serving and replay.
	store := so.GetBlockStore()
	a := &Adapter{
		le:                  le,
		opened:              make(chan struct{}),
		so:                  so,
		opts:                opts,
		bucketID:            store.GetID(),
		cursors:             cursorcodec.New(le, opts.Bus, opts.StepFactorySet, store.GetDecodedBlockCache(), !opts.transient),
		pages:               block.NewPendingStore(store, opts.MaxPendingBytes),
		queue:               &opQueue{so: so},
		missing:             make(map[string]struct{}),
		legacyReplayMatcher: os.Getenv(ReplayIdentityEnv) == "off",
		expectWrites:        os.Getenv(ReplayIdentityEnv) != "off" && os.Getenv(ExpectWriteEnv) != "off",
		orderedHolds:        os.Getenv(OrderedHoldEnv) != "off" && (sobject.OrdersBlockWrites(so) || forceOrderedHolds.Load()),
		fences:              make(map[string]*spaceFence),
		held:                make(map[string]*block.BlockRef),

		cursorWorlds:     rootSet{name: RootReplayCursor},
		pendingWorlds:    rootSet{name: RootPending},
		checkpointWorlds: rootSet{name: RootCheckpoint},
		imported:         rootSet{name: RootImport},
		payloads:         rootSet{name: RootReplayPayloads},
		payloadVersions:  make(map[string]uint32),
		replayState:      newReplayOpening(),
		retained:         newRetainedHold(),
		settles:          settles,
	}
	a.pages.SetKeepStaged(keepStaged)
	a.lookupOp = a.buildLookupOp()
	a.replay = &spaceReplay{a: a}
	a.backfill = newSpaceBackfill(a)

	// Let transient replay run immediately under its caller-owned stage epoch.
	if opts.transient {
		a.replay.stage = newReplayStage(a.pages)
		close(a.opened)
	}
	return a, nil
}

// Execute claims the Space, opens its held state, and serves its World until
// ctx ends. It drains body calls and background stores before releasing the claim.
func (a *Adapter) Execute(ctx context.Context) error {
	// Claim the Space after the previous body drains; detach runs last.
	stopBody, detach, err := a.so.SetBody(ctx, a)
	if err != nil {
		return err
	}
	defer detach()
	runCtx, stopRun := context.WithCancel(ctx)
	a.runCtx = runCtx

	// Drain startup and serving work before releasing replay resources.
	defer func() {
		// Join every driver and voter call while retaining the body's claim.
		stopBody()

		// Refuse new stores, cancel the running stores and drain their holds.
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			a.stopped = true
		})
		stopRun()
		a.waitBackground()

		// End replay only after its last user has returned.
		a.replay.close()
		a.replay.stage.end()
	}()

	// Read what an earlier run held, then let the driver step the body.
	if err := a.loadRoots(ctx); err != nil {
		return err
	}
	if err := a.loadCursor(ctx); err != nil {
		a.le.WithError(err).Warn("saved replay cursor is not readable, replaying from the checkpoint")
	}
	if err := a.backfill.start(ctx); err != nil {
		return err
	}
	a.replay.stage = newReplayStage(a.pages)

	// Bound absence-based settlement before replay can adopt additional writes.
	a.settleSeq = a.settles.last()
	close(a.opened)

	// Run admission, reclaim and settlement of earlier unknown writes.
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return a.queue.execute(ctx, a.queued) })
	g.Go(func() error { return a.executeStorageReclaim(ctx) })
	g.Go(func() error { return a.adoptSettles(ctx) })
	g.Go(func() error { return a.executeRetainedRetry(ctx) })

	// Serve the engine once the World is initialized.
	g.Go(func() error {
		// Build and publish the engine.
		engine, err := a.serve(ctx)
		if err != nil {
			return err
		}

		// Retire the engine and supersede its open transactions when the run ends.
		<-ctx.Done()
		a.endAdmissions()
		a.retire()
		err = engine.retire()
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			a.readers = nil
		})
		return err
	})
	err = g.Wait()
	return err
}

// errAdapterStopped is returned by a background store asked of an adapter
// whose run ended.
var errAdapterStopped = errors.New("space world adapter stopped")

// startBackground keeps roots live until the idempotent done runs. Execute
// waits for these holds. After Execute stops, it returns false without a hold.
func (a *Adapter) startBackground(roots ...*block.BlockRef) (done func(), ok bool) {
	// Count the store and keep its roots live, unless the run ended.
	var token uint64
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Refuse a store once the run ended, before Execute waits.
		if a.stopped {
			return
		}

		// Register it under its token.
		if a.storing == nil {
			a.storing = make(map[uint64][]*block.BlockRef)
		}
		token, ok = a.storingNext, true
		a.storingNext++
		a.storing[token] = slices.Clone(roots)
	})
	if !ok {
		return func() {}, false
	}

	// Drop them once, waking Execute's wait.
	return func() {
		a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if _, ok := a.storing[token]; ok {
				delete(a.storing, token)
				broadcast()
			}
		})
	}, true
}

// waitBackground waits until every background store ended.
func (a *Adapter) waitBackground() {
	_ = a.bcast.Wait(context.Background(), func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return len(a.storing) == 0, nil
	})
}

// backgroundContext ends with ctx or Execute. Its ordered root writes may be
// lost on a crash: replay repeats them from the durable operation log.
func (a *Adapter) backgroundContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(kvtx.WithOrderedCommitContext(ctx))
	if a.runCtx != nil {
		stop := context.AfterFunc(a.runCtx, cancel)
		return ctx, func() {
			stop()
			cancel()
		}
	}
	return ctx, cancel
}

// waitOpened waits for Execute's claim and loaded roots.
func (a *Adapter) waitOpened(ctx context.Context) error {
	select {
	case <-a.opened:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// endAdmissions supersedes unsent writes and reports other undecided writes as
// durability unknown. Adopted writes stay unsettled for the next adapter.
func (a *Adapter) endAdmissions() {
	// Supersede the ones the queue never passed on.
	waiting := a.queue.drain()
	for _, adm := range waiting {
		a.fail(adm, world.ErrSuperseded)
	}

	// Leave the rest unknown.
	var rest []*spaceAdmission
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rest = slices.DeleteFunc(slices.Clone(a.pending), func(adm *spaceAdmission) bool { return adm.settle != nil })
	})
	for _, adm := range rest {
		a.fail(adm, durabilityUnknown(adm.receipt.OpHash(), errAdapterStopped))
	}
}

// AcquireEngine holds the serving engine open until release. Release after
// WaitEngineReplaced and write through another reference: a write on a reloading
// adapter waits for all holds, including the caller's own hold.
func (a *Adapter) AcquireEngine(ctx context.Context) (*Engine, func(), error) {
	// Hold the engine once one serves.
	var engine *Engine
	err := a.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if engine = a.engine; engine != nil {
			a.holders++
		}
		return engine != nil, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return engine, a.releaseEngine, nil
}

// releaseEngine drops one AcquireEngine hold.
func (a *Adapter) releaseEngine() {
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.holders--
		broadcast()
	})
}

// WaitEngineReplaced waits until engine no longer serves.
func (a *Adapter) WaitEngineReplaced(ctx context.Context, engine *Engine) error {
	return a.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return a.engine != engine, nil
	})
}

// retire stops serving the engine and waits for every hold on it.
func (a *Adapter) retire() {
	_ = a.bcast.Wait(context.Background(), func(broadcast func(), _ func() <-chan struct{}) (bool, error) {
		if a.engine != nil {
			a.engine = nil
			broadcast()
		}
		return a.holders == 0, nil
	})
}

// serve initializes an empty World as an owner and publishes its engine after
// replay reaches the opening order or waits for a missing handler.
func (a *Adapter) serve(ctx context.Context) (*Engine, error) {
	// Capture the opening order to prevent serving older World state.
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	target, err := sobject.ReadOrder(ctx, snap)
	if err != nil {
		return nil, err
	}

	// Serve reads at the opening order or a handler wait; writes still gate.
	var queued bool
	for {
		var installed, reached, waiting bool
		var state *InnerState
		var waitCh <-chan struct{}
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			installed, state, waitCh = a.installed, a.headState, getWaitCh()
			reached = a.reachedLocked(target)
			waiting = a.replayState.GetWaiting() != nil
		})
		if !state.GetHeadRef().GetEmpty() && (reached || waiting) {
			break
		}

		// Initialize a World that replay of the opening order left empty.
		if installed && reached && !queued {
			init, err := a.initOp(ctx)
			if err != nil {
				return nil, err
			}
			if init != nil {
				// Bypasses the write gate, as enqueue says.
				if _, err := a.enqueueOp(init); err != nil {
					return nil, err
				}
			}
			queued = true
		}
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-waitCh:
		}
	}

	// Build the engine at the installed head.
	var state *InnerState
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = a.headState
	})

	// Choose the engine's options.
	engineOpts := []world_block.EngineOption{world_block.WithRecordedOperations()}
	if a.opts.ReaderPinDelay != nil {
		engineOpts = append(engineOpts, world_block.WithReaderPinDelay(*a.opts.ReaderPinDelay))
	}

	// Open the engine there and publish it.
	cursor, err := a.newCursor(ctx, a.pages, state.GetHeadRef())
	if err != nil {
		return nil, err
	}
	bengine, err := world_block.NewEngine(ctx, a.le, cursor, a.lookupOp, a, a.opts.Verbose, engineOpts...)
	if err != nil {
		cursor.Release()
		return nil, err
	}
	engine := &Engine{Engine: bengine, spaceBackfill: a.backfill, a: a, cursor: cursor}
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.engine, a.readers = engine, engine
		broadcast()
	})
	return engine, nil
}

// reachedLocked reports whether the installed head is at or past target's
// order. The caller holds bcast.
func (a *Adapter) reachedLocked(target *sobject.OrderEvent) bool {
	return a.headHeight > target.Checkpoint.GetHeight() ||
		(a.headHeight == target.Checkpoint.GetHeight() && a.headPositions >= len(target.Order))
}

// initOp returns the operation that initializes the World when this device
// owns the Space, or nil.
func (a *Adapter) initOp(ctx context.Context) (*WorldOp, error) {
	// Only an owner that is not handing over initializes.
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	own, err := snap.GetOwnership(ctx)
	if err != nil {
		return nil, err
	}
	if !own.Owns(a.so.GetPeerID().String()) {
		return nil, nil
	}

	// Build it.
	initOp, err := NewInitWorldOp(a.opts.InitWorldOp)
	if err != nil {
		return nil, err
	}
	return &WorldOp{Body: &WorldOp_InitWorld{InitWorld: initOp}}, nil
}

// Head returns the installed head.
func (a *Adapter) Head(context.Context) (world.Head, error) {
	var head world.Head
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		head = a.head
	})
	return head, nil
}

// WatchHead calls fn with the installed head, then with each later installed
// head, until release is called. Every head is durable. The adapter calls fn
// before it decides the candidates the head holds.
func (a *Adapter) WatchHead(_ context.Context, fn func(world.Head)) (func(), error) {
	// Register and report the current head, serialized with later reports.
	w := &headWatch{fn: fn}
	a.watchMtx.Lock()
	var head world.Head
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.watchers = append(a.watchers, w)
		head = a.head
	})
	if !head.Root.GetEmpty() {
		fn(head)
	}
	a.watchMtx.Unlock()

	// Release removes the registration.
	return func() {
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			a.watchers = slices.DeleteFunc(a.watchers, func(o *headWatch) bool { return o == w })
		})
	}, nil
}

// Begin takes the local writer turn at the provisional writer head. It refuses
// nonwriters and departing owners, waits for opening replay, and returns
// *sobject.WriteWait while replay waits.
func (a *Adapter) Begin(ctx context.Context) (world.Turn, error) {
	// Gate before reading the role so reload yields the retryable retirement
	// error, rather than an ended mount's state-read error.
	if err := a.gateWrite(ctx, nil); err != nil {
		return nil, err
	}

	// Refuse a participant that cannot write.
	if err := a.checkWrites(ctx); err != nil {
		return nil, err
	}

	// Take the turn at the writer head.
	release, err := a.turnMtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	t := &spaceTurn{a: a, release: release}
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		t.base = a.writer
		t.base.Durable = t.base.Root.EqualsRef(a.head.Root) && t.base.Seqno == a.head.Seqno
		t.after, t.afterHead = a.writerAdmissionLocked(), a.headState.lastWrite(a.so.GetPeerID().String())
	})
	return t, nil
}

// checkWrites refuses nonwriters and departing owners with tx.ErrNotWrite.
func (a *Adapter) checkWrites(ctx context.Context) error {
	// The role must let the participant write.
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	participant, err := snap.GetParticipantConfig(ctx)
	if err != nil {
		return err
	}
	if !sobject.CanWriteOps(participant.GetRole()) {
		return tx.ErrNotWrite
	}

	// A departing owner writes nothing more.
	own, err := a.ownershipOf(ctx, snap)
	if err != nil {
		return err
	}
	if own.Departing(a.so.GetPeerID().String()) {
		return tx.ErrNotWrite
	}
	return nil
}

// ownershipOf caches ownership by config head.
func (a *Adapter) ownershipOf(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*sobject.SOOwnership, error) {
	// Reuse the ownership of an unchanged head.
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	head := cfg.GetConfigChainHash()
	var own *sobject.SOOwnership
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if len(head) != 0 && bytes.Equal(a.ownershipHead, head) {
			own = a.ownership
		}
	})
	if own != nil {
		return own, nil
	}

	// Derive it and keep it for the head.
	own, err = snap.GetOwnership(ctx)
	if err != nil {
		return nil, err
	}
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.ownership, a.ownershipHead = own, bytes.Clone(head)
	})
	return own, nil
}

// Root returns the encoded World after n positions and a store function.
// The World stays live until store returns. Storage flushes pages outside the
// root lock, then holds the World beside earlier returns since the last install.
func (a *Adapter) Root(ctx context.Context, n int) ([]byte, func(context.Context) error, error) {
	// Wait for the claim.
	if err := a.waitOpened(ctx); err != nil {
		return nil, nil, err
	}

	// Answer only for an applied position, readable from memory.
	r := a.replay
	if n > len(r.positions) {
		return nil, nil, nil
	}
	if n != 0 && n <= r.floor {
		// The World after n left memory unstored: compute it again.
		if err := r.recompute(ctx); err != nil {
			return nil, nil, err
		}
	}
	state, _ := r.stateBefore(n)
	data, err := state.MarshalVT()
	if err != nil {
		return nil, nil, err
	}

	// Keep the World live until its store returns.
	root := state.GetHeadRef().GetRootRef().Clone()
	done, ok := a.startBackground(root)
	if !ok {
		return nil, nil, errAdapterStopped
	}
	store := func(ctx context.Context) error {
		// End with the adapter's run, and free the World's liveness after.
		defer done()
		ctx, cancel := a.backgroundContext(ctx)
		defer cancel()

		// Let the test hold a page store before it touches the root lock.
		if hook := checkpointStoreHook.Load(); hook != nil {
			(*hook)(ctx)
		}

		// Hold the checkpoint World unless replay-base already holds it.
		a.cursors.CountStoredPages(ctx, a.pages, root)
		release, err := a.lockRoots(ctx, root)
		if err != nil {
			return err
		}
		defer release()
		return a.retryMovedLocked(ctx, func() error {
			if refEqual(a.held[RootReplayBase], root) {
				return nil
			}
			return a.addRoot(ctx, &a.checkpointWorlds, root)
		})
	}
	return data, store, nil
}

// Apply reuses a matching local candidate or replays op at index. It publishes
// the resulting head only after replay accounts for every previously applied
// operation, then decides the candidates that head holds.
func (a *Adapter) Apply(ctx context.Context, index int, op *sobject.OrderedOperation) (string, error) {
	// Wait for the claim.
	if err := a.waitOpened(ctx); err != nil {
		return "", err
	}

	// Note the applied operations a reorder put back.
	r := a.replay
	for _, pos := range r.positions[index:] {
		if pos.reason == "" {
			a.missing[string(pos.op.Hash)] = struct{}{}
		}
	}
	r.positions = r.positions[:index]

	// Reuse the local candidate only on its original predecessor.
	adm := a.match(op)
	prev, _ := r.stateBefore(index)
	var adopt *spacePosition
	expected := adm.getExpected()

	// Adopt a candidate whose base and expected write still match.
	if c := adm.getCandidate(); c != nil && op.Refused == "" && refEqual(c.Base, prev.GetHeadRef().GetRootRef()) && prev.expects(op.Inner.GetPeerId(), expected) {
		state := prev.CloneVT()
		state.HeadRef.RootRef = c.Root.Clone()
		state.HeadRef.BucketId = ""
		if expected != nil {
			state = state.withLastWrite(&LastWrite{PeerId: op.Inner.GetPeerId(), OpHash: op.Hash})
		}
		adopt = &spacePosition{state: state, seqno: c.Seqno, payloads: adm.payloads, handlers: adm.handlers}
		if replayCheck {
			r.checkAdopted(ctx, index, op, adopt)
		}
	}

	// Replay with resumable handler, upgrade and storage waits. Transient
	// replay returns failures directly because it has no waiting driver.
	pos, err := r.run(ctx, index, op, adopt)
	var unresolved *unresolvedOpError
	switch {
	case errors.As(err, &unresolved) && a.opts.transient:
		return "", errors.Wrap(ErrUnresolvedOp, unresolved.opTypeID)
	case errors.As(err, &unresolved):
		return "", a.waitForHandler(unresolved)
	case errors.Is(err, ErrUnreadableOp) && !a.opts.transient:
		upgrade := &sobject.SOReplayWait{State: &sobject.SOReplayWait_Upgrade{Upgrade: &sobject.SOReplayWaitUpgrade{}}}
		return "", &sobject.BodyWait{Wait: upgrade, Wake: neverWakes, Err: err}
	case err != nil && !a.opts.transient && ctx.Err() == nil && isReplayGap(err):
		gap := &sobject.SOReplayWait{State: &sobject.SOReplayWait_Gap{Gap: &sobject.SOReplayWaitGap{}}}
		return "", &sobject.BodyWait{Wait: gap, Err: err}
	case err != nil:
		return "", err
	}
	r.positions = append(r.positions, pos)
	delete(a.missing, string(op.Hash))

	// Hold remote payloads so a successor can replay without their author.
	if len(pos.payloads) != 0 {
		if err := a.addPayloads(ctx, pos.payloads, false); err != nil {
			return "", err
		}
	}

	// Publish decisions only after accounting for missing operations.
	if adm != nil {
		a.place(adm, pos, adopt == nil)
	} else if a.legacyReplayMatcher {
		a.noteUnmatched(op, pos)
	}
	if err := a.noteFence(ctx, op, pos); err != nil {
		return "", err
	}
	if len(a.missing) == 0 {
		if err := a.installHead(ctx); err != nil {
			return "", err
		}
	}

	// Save the cursor when it falls behind.
	if err := a.maybeSaveCursor(ctx); err != nil {
		return "", err
	}
	return pos.reason, nil
}

// Install adopts ev's checkpoint, retains matching replay positions above it,
// and reconciles admission coverage and missing operations from the same event.
func (a *Adapter) Install(ctx context.Context, ev *sobject.OrderEvent, covered int, held *sobject.SOOperationSet) (bool, error) {
	// Wait for the claim.
	if err := a.waitOpened(ctx); err != nil {
		return false, err
	}
	checkpoint := ev.Checkpoint

	// Decode the checkpoint's World.
	r := a.replay
	base := &InnerState{}
	if err := base.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		return false, errors.Wrap(err, "checkpoint World state")
	}

	// Read its seqno.
	seqno, err := r.seqno(ctx, base)
	if err != nil {
		return false, err
	}

	// Retain the replay suffix only when its predecessor matches the checkpoint.
	kept := false
	if covered >= 0 && covered <= len(r.positions) {
		state, _ := r.stateBefore(covered)
		kept = state.EqualVT(base)
		if replayCheck && !kept {
			r.logInstallMismatch(checkpoint.GetHeight(), covered, base, state)
		}
	}
	paid := kept && slices.ContainsFunc(r.positions[:covered], func(pos spacePosition) bool { return len(pos.payloads) != 0 })
	inCheckpoint := a.dropPositions(covered, kept, ev.Set.Get)
	r.base, r.baseSeqno, r.height = base, seqno, checkpoint.GetHeight()
	if len(r.restored) != 0 && !kept && !r.restoredFrom.EqualVT(base) {
		r.restored = nil
	}

	// Release covered payloads while preserving those still above the checkpoint.
	if paid {
		if err := a.resetPayloads(ctx, true); err != nil {
			return false, err
		}
	}

	// Schedule the atomic replay-base and history root move.
	if err := a.trimInBackground(ctx); err != nil {
		return false, err
	}

	// Reconcile checkpoint coverage before dropping unreachable buffered pages.
	var refused, unknown []*spaceAdmission
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.installed, a.checkpoint, a.checkpointSeqno = true, base, seqno
		refused, unknown = a.installCoverLocked(checkpoint, inCheckpoint, r.positions, a.so.GetPeerID().String(), held.Get)
		broadcast()
	})
	a.decideCovered(refused, unknown)
	a.sweep()
	if len(a.missing) == 0 {
		if err := a.installHead(ctx); err != nil {
			return false, err
		}
	}
	return kept, nil
}

// trimPlan is what a trim gathered on the driver goroutine: the base, the
// live Worlds, and the Worlds each history held then, the only ones the trim
// may release.
type trimPlan struct {
	// base is the replay base's World.
	base *block.BlockRef
	// memory and candidates are the live Worlds the trim holds under the
	// cursor and pending roots, without the base.
	memory, candidates []*block.BlockRef
	// live holds the keys of every live World.
	live map[string]struct{}
	// held holds the keys of the Worlds the histories held at the gather.
	held map[string]struct{}
	// seq numbers the plan in gather order.
	seq uint64
}

// releases permits dropping only roots held at gather time that became the
// base or ceased to be live. Roots gained after the gather stay held.
func (p *trimPlan) releases(root *block.BlockRef) bool {
	key := root.MarshalString()
	if _, ok := p.held[key]; !ok {
		return false
	}
	_, live := p.live[key]
	return !live || refEqual(root, p.base)
}

// gatherTrim gathers a trim's plan. The driver goroutine calls it.
func (a *Adapter) gatherTrim(ctx context.Context) (*trimPlan, error) {
	// Gather the live Worlds, leaving out the base.
	l := a.gatherLive()
	p := &trimPlan{base: a.replay.base.GetHeadRef().GetRootRef(), seq: a.trimSeq.Add(1)}
	isBase := func(root *block.BlockRef) bool { return refEqual(root, p.base) }
	p.memory = slices.DeleteFunc(dedupRefs(l.memory), isBase)
	p.candidates = slices.DeleteFunc(dedupRefs(l.candidates), isBase)
	p.live = make(map[string]struct{}, len(p.memory)+len(p.candidates)+len(l.stored))
	for _, root := range slices.Concat(p.memory, p.candidates, l.stored) {
		p.live[root.MarshalString()] = struct{}{}
	}

	// Note the Worlds the histories hold now.
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	p.held = make(map[string]struct{})
	for _, h := range []*rootSet{&a.cursorWorlds, &a.pendingWorlds, &a.checkpointWorlds} {
		for _, root := range h.roots {
			p.held[root.MarshalString()] = struct{}{}
		}
	}
	return p, nil
}

// trimBackoffMin and trimBackoffMax bound the waits between the attempts of
// a background trim whose root move failed.
const (
	trimBackoffMin = 250 * time.Millisecond
	trimBackoffMax = 30 * time.Second
)

// trimInBackground runs the newest gathered trim beside apply. Trims serialize
// and retry with backoff. Until its root move, the checkpoint root holds the
// adopted base and replay-base holds the previous one.
func (a *Adapter) trimInBackground(ctx context.Context) error {
	// Gather the plan on the driver goroutine.
	plan, err := a.gatherTrim(ctx)
	if err != nil {
		return err
	}

	// Keep the plan's Worlds live while its background flush runs.
	done, ok := a.startBackground(slices.Concat(plan.memory, plan.candidates, []*block.BlockRef{plan.base})...)
	if !ok {
		return nil
	}
	go func() {
		defer done()
		ctx, cancel := a.backgroundContext(ctx)
		defer cancel()
		a.runTrimNewest(ctx, plan)
	}()
	return nil
}

// runTrimNewest runs plan under the trim turn while it is the newest plan
// gathered, trying again with backoff after a failure until ctx ends.
func (a *Adapter) runTrimNewest(ctx context.Context, plan *trimPlan) {
	delay := trimBackoffMin
	for {
		// Run it under the trim turn unless a newer plan was gathered.
		release, err := a.trimMtx.Lock(ctx)
		if err != nil {
			return
		}
		if plan.seq < a.trimSeq.Load() {
			release()
			return
		}
		err = a.runTrim(ctx, plan)
		release()
		if err == nil || ctx.Err() != nil {
			return
		}

		// Wait out the backoff, then try again.
		a.le.WithError(err).Warn("histories not trimmed after the install, trying again")
		select {
		case <-ctx.Done():
			return
		//gostyle:timer backoff
		case <-time.After(delay):
		}
		delay = min(2*delay, trimBackoffMax)
	}
}

// runTrim atomically replaces released history roots with the live Worlds.
// In-memory Worlds may reference stored pages held only by those histories, so
// the same move stores and holds them under cursor or pending. Stored Worlds
// stay in their histories. Imported roots leave only when held elsewhere.
// Cost per install: one indexed pass over live Worlds and one root move.
func (a *Adapter) runTrim(ctx context.Context, p *trimPlan) error {
	// Take the root lock, keeping what the plan does not release.
	keep := func(root *block.BlockRef) bool { return !p.releases(root) }
	reach := slices.Concat(p.memory, p.candidates, []*block.BlockRef{p.base})
	a.cursors.CountStoredPages(ctx, a.pages, reach...)
	release, err := a.lockRoots(ctx, reach...)
	if err != nil {
		return err
	}
	defer release()
	return a.retryMovedLocked(ctx, func() error {
		// Read the roots an import appended since the last move.
		if err := a.readImportLocked(ctx); err != nil {
			return err
		}

		// Release imports only from current holds under rootMtx, never from a
		// plan that predates an import append.
		keepImported := func(root *block.BlockRef) bool {
			return !refEqual(root, p.base) && !a.payloads.holds(root)
		}
		sets := []*rootSet{&a.cursorWorlds, &a.pendingWorlds, &a.checkpointWorlds, &a.imported}
		moved := !refEqual(a.held[RootReplayBase], p.base)
		if !moved && !slices.ContainsFunc(sets[:3], func(h *rootSet) bool { return h.releases(keep) }) && !a.imported.releases(keepImported) {
			return nil
		}

		// Hold the live Worlds, release the rest and move the base together.
		next := [][]*block.BlockRef{
			a.cursorWorlds.held(keep, p.memory),
			a.pendingWorlds.held(keep, p.candidates),
			a.checkpointWorlds.held(keep, nil),
			a.imported.held(keepImported, nil),
		}
		return a.setRootSets(ctx, sets, next, nil, false, block.NamedRoot{Name: RootReplayBase, Ref: p.base})
	})
}

// dropPositions keeps the suffix after covered when its World matches the
// checkpoint, returning the covered prefix. Otherwise it drops every position.
// It reconciles missing operations against above in either case.
func (a *Adapter) dropPositions(covered int, kept bool, above func(h []byte) *sobject.SOOperationInner) []spacePosition {
	// Retain the uncovered suffix and reconcile missing operations.
	r := a.replay
	if kept {
		inCheckpoint := r.positions[:covered]
		r.positions = slices.Clone(r.positions[covered:])
		r.floor, r.saved = max(0, r.floor-covered), max(0, r.saved-covered)
		a.noteMissing(nil, above)
		return inCheckpoint
	}

	// Drop them all, noting those replay applies again.
	a.noteMissing(r.positions, above)
	r.positions, r.floor, r.saved = nil, 0, 0
	return nil
}

// noteMissing tracks dropped applied operations still above the checkpoint.
// Only replay or coverage removes a missing hash. above must come from the same
// OrderEvent as the checkpoint: a later event could cover an operation this
// checkpoint lacks and let installHead publish a head that lost it.
func (a *Adapter) noteMissing(dropped []spacePosition, above func(h []byte) *sobject.SOOperationInner) {
	for _, pos := range dropped {
		if pos.reason == "" && above(pos.op.Hash) != nil {
			a.missing[string(pos.op.Hash)] = struct{}{}
		}
	}
	for key := range a.missing {
		if above([]byte(key)) == nil {
			delete(a.missing, key)
		}
	}
}

// newAdmission creates an operation admission with an optional local candidate.
func (a *Adapter) newAdmission(candidate world.Candidate, op []byte, payloads []*block.BlockRef, handlers *PayloadHandlers) *spaceAdmission {
	return &spaceAdmission{a: a, candidate: candidate, op: op, payloads: payloads, handlers: handlers, done: make(chan struct{})}
}

// enqueue queues op after admitted writes; nil is an acknowledgment.
// It bypasses gateWrite because its callers do not choose edits from the head:
// initialization, acknowledgments, fences and root changes replay independently
// of the writer's base. Gating them could stall initialization or reclaim.
func (a *Adapter) enqueue(op []byte) *spaceAdmission {
	adm := a.newAdmission(world.Candidate{}, op, nil, nil)
	a.admit(adm)
	a.queue.push(adm)
	return adm
}

// enqueueOp encodes op and enqueues it.
func (a *Adapter) enqueueOp(op *WorldOp) (*spaceAdmission, error) {
	data, err := op.MarshalVT()
	if err != nil {
		return nil, err
	}
	return a.enqueue(data), nil
}

// admit records adm as pending and makes its candidate the writer head.
func (a *Adapter) admit(adm *spaceAdmission) {
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.admitLocked(adm)
	})
}

// admitWrite checks replay readiness and admits adm under the same bcast hold,
// so replay cannot start waiting between the check and admission.
func (a *Adapter) admitWrite(ctx context.Context, adm *spaceAdmission) error {
	return a.gateWrite(ctx, func() { a.admitLocked(adm) })
}

// admitLocked records adm as pending and makes its candidate the writer head.
// The caller holds bcast.
func (a *Adapter) admitLocked(adm *spaceAdmission) {
	a.pending = append(a.pending, adm)
	if !adm.candidate.Root.GetEmpty() {
		a.writer = world.Head{Root: adm.candidate.Root, Seqno: adm.candidate.Seqno}
	}
}

// checkpointStoreHook lets a test park checkpoint page storage before it
// takes the root lock. A production adapter never sets the hook.
var checkpointStoreHook atomic.Pointer[func(context.Context)]

// admittedHook, when set, sees each candidate keepAdmitted keeps, so a test can
// check what its writer reported.
var admittedHook atomic.Pointer[func(world.Candidate)]

// heldHook, when set, runs after each hold keepAdmitted takes, with whether the
// hold was ordered, and fails the admission with its error, so a test can
// abandon a write between its hold and its queued state write.
var heldHook atomic.Pointer[func(ordered bool) error]

// forceOrderedHolds, set only by tests, orders the holds of an Adapter opened
// on a SharedObject that does not report ordering its block store's writes.
var forceOrderedHolds atomic.Bool

// keepAdmitted keeps candidate blocks and holds their roots before queueing.
// Overflow blocks stay staged through holdAdmitted; drained writer blocks stay
// staged through transaction end. In-memory puts do not open an overflow stage.
func (a *Adapter) keepAdmitted(ctx context.Context, adm *spaceAdmission) error {
	// Show the candidate to a test that asked to see it.
	if hook := admittedHook.Load(); hook != nil {
		(*hook)(adm.candidate)
	}
	overflow := a.pages.OpenOverflowStage()
	defer overflow.Release()
	if _, err := overflow.PutBlockBatch(ctx, adm.candidate.Entries); err != nil {
		return err
	}
	if err := a.holdAdmitted(ctx, adm, overflow.Stored()); err != nil {
		return err
	}

	// Let a test abandon the write after its hold.
	if hook := heldHook.Load(); hook != nil {
		return (*hook)(a.orderedHolds)
	}
	return nil
}

// holdAdmitted holds payloads and spilled candidate Worlds before queueing.
// Each swap stores only the writer's pages and takes rootMtx only for the swap.
//
// A hold must be durable by its operation's state write. With orderedHolds,
// BlockWriteOrderer keeps a crash prefix: a lost hold also loses its operation.
// Without ordering, each swap is durable on return. Replay and reset holds cover
// already durable operations and therefore always commit durably. Background
// World holds can commit ordered because replay can rebuild them.
func (a *Adapter) holdAdmitted(ctx context.Context, adm *spaceAdmission, stored bool) error {
	// Order the swaps behind the operation's state write when it orders them.
	if a.orderedHolds {
		ctx = kvtx.WithOrderedCommitContext(ctx)
	}

	// Protect payloads before queueing their operation.
	if len(adm.payloads) != 0 {
		if err := a.addPayloads(ctx, adm.payloads, true); err != nil {
			return err
		}
	}

	// Protect spilled candidates beside earlier pending Worlds.
	if (!stored && !adm.candidate.Staged && !a.pages.Full()) || adm.candidate.Root.GetEmpty() {
		return nil
	}
	a.cursors.CountStoredPages(ctx, a.pages, adm.candidate.Root)
	release, err := a.lockRoots(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Kept pages, open stages and live-World holds protect these blocks.
	// RequireHeld would refuse revived boundary blocks during their grace.
	return a.addRoot(ctx, &a.pendingWorlds, adm.candidate.Root)
}

// match finds the pending admission of this exact signed operation. Neither
// identical WorldOp bytes nor a reused local ID proves replay attribution.
func (a *Adapter) match(op *sobject.OrderedOperation) *spaceAdmission {
	// Only this device's operations can match its pending admissions.
	if op.Inner.GetPeerId() != a.so.GetPeerID().String() {
		return nil
	}

	// Use receipts for exact attribution, retaining the legacy fallback.
	var out *spaceAdmission
	if a.legacyReplayMatcher {
		out = a.matchLegacy(op)
	} else {
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			for _, adm := range a.pending {
				if adm.receipt.MatchesHash(op.Hash) {
					adm.hash = slices.Clone(op.Hash)
					out = adm
					return
				}
			}
		})
	}
	if out != nil {
		return out
	}

	// Settle a write another adapter sent or left unknown.
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		out = a.adoptReplayedLocked(op)
	})
	return out
}

// matchLegacy is the matcher used before the signer published receipts.
func (a *Adapter) matchLegacy(op *sobject.OrderedOperation) *spaceAdmission {
	// Body-less refusals use the callback identity at noteUnmatched.
	if !refusalHandoffOff && op.Refused != "" {
		return nil
	}

	// Match a previously placed hash or a candidate still waiting for replay.
	var out *spaceAdmission
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, adm := range a.pending {
			if adm.hash != nil {
				if bytes.Equal(adm.hash, op.Hash) {
					out = adm
					return
				}
				continue
			}
			localMatch := adm.localID != "" && adm.localID == op.Inner.GetLocalId()
			dataMatch := adm.localID == "" && op.Data != nil && bytes.Equal(adm.op, op.Data)
			if adm.state == admissionQueued && adm.op != nil && (localMatch || dataMatch) {
				adm.hash = slices.Clone(op.Hash)
				out = adm
				return
			}
		}
	})
	return out
}

// unmatchedRefusalKey uses signed hashes, or fork-ambiguous local IDs in legacy mode.
func unmatchedRefusalKey(localID string, hash []byte) string {
	if refusalHandoffOff {
		return localID
	}
	return string(hash)
}

// noteUnmatched hands a legacy refusal to its matching admission or caches it
// for the queue callback, whichever acquires bcast first.
func (a *Adapter) noteUnmatched(op *sobject.OrderedOperation, pos spacePosition) {
	// A local refusal must name an exact operation.
	if pos.reason == "" || op.Inner.GetPeerId() != a.so.GetPeerID().String() {
		return
	}
	localID := op.Inner.GetLocalId()
	if !refusalHandoffOff && (localID == "" || len(op.Hash) == 0) {
		return
	}

	// Find the admission or leave the refusal in the same lock section that
	// queued uses to publish the operation's identity and check the cache.
	var target *spaceAdmission
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// The queue callback may already have returned since Apply looked.
		if !refusalHandoffOff {
			for _, adm := range a.pending {
				if adm.localID == localID && bytes.Equal(adm.pos.GetOpHash(), op.Hash) {
					target = adm
					return
				}
			}
		}

		// Let the later queue callback consume the refusal if it needs it.
		if a.unmatched == nil {
			a.unmatched = make(map[string]spacePosition)
		}
		a.unmatched[unmatchedRefusalKey(localID, op.Hash)] = pos
	})

	// Refuse only the matching admission, outside the broadcast lock.
	if target != nil {
		a.rejectAdmission(target, pos.reason)
	}
}

// rejectAdmission reports this operation's replay refusal to its waiter.
func (a *Adapter) rejectAdmission(adm *spaceAdmission, reason string) {
	a.decide([]*spaceAdmission{adm}, world.Rejected, reason, errors.Wrap(sobject.ErrRejectedOp, reason))
}

// getCandidate returns the admission's candidate, or nil for none.
func (s *spaceAdmission) getCandidate() *world.Candidate {
	if s == nil || s.candidate.Root.GetEmpty() {
		return nil
	}
	return &s.candidate
}

// place defers applied admissions until head publication. Refusals supersede a
// candidate replayed on a changed base and reject other operations.
func (a *Adapter) place(adm *spaceAdmission, pos spacePosition, ranAgain bool) {
	// Keep an applied operation for the install.
	if pos.reason == "" {
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if adm.state == admissionQueued {
				adm.state = admissionPlaced
			}
		})
		return
	}

	// Decide a refused one.
	decision := world.Rejected
	var err error
	if ranAgain && pos.op.Refused == "" && adm.getCandidate() != nil {
		decision = world.Superseded
	} else {
		err = errors.Wrap(sobject.ErrRejectedOp, pos.reason)
	}
	a.decide([]*spaceAdmission{adm}, decision, pos.reason, err)
}

// installCoverLocked reconciles placed admissions, proves checkpoint coverage
// from held, and returns refused or unknown admissions for decisions outside
// bcast. The caller holds bcast.
func (a *Adapter) installCoverLocked(
	checkpoint *sobject.SOCheckpointInner,
	inCheckpoint, kept []spacePosition,
	peerID string,
	held func(h []byte) *sobject.SOOperationInner,
) (refused, unknown []*spaceAdmission) {
	a.reconcilePlacedLocked(inCheckpoint, kept)
	a.cover = nextCover(a.cover, checkpoint, peerID, held)
	for _, adm := range a.pending {
		switch a.coverLocked(adm) {
		case coverRefused:
			refused = append(refused, adm)
		case coverUnknown:
			unknown = append(unknown, adm)
		}
	}
	return refused, unknown
}

// coverLocked applies checkpoint coverage to a queued admission with a known
// position. Applied admissions wait for installHead. The caller holds bcast.
func (a *Adapter) coverLocked(adm *spaceAdmission) coverOutcome {
	if adm.state != admissionQueued {
		return coverNone
	}
	outcome := a.cover.outcome(adm.pos)
	if outcome == coverApplied {
		adm.state = admissionCovered
	}
	return outcome
}

// reconcilePlacedLocked preserves evidence for placed admissions: covered
// positions become covered, retained positions stay placed, and others requeue.
// Only replay positions, missing operations or checkpoint proof can justify
// acceptance by installHead. The caller holds bcast.
func (a *Adapter) reconcilePlacedLocked(inCheckpoint, kept []spacePosition) {
	holds := func(positions []spacePosition, hash []byte) bool {
		return slices.ContainsFunc(positions, func(pos spacePosition) bool {
			return pos.reason == "" && bytes.Equal(pos.op.Hash, hash)
		})
	}
	for _, adm := range a.pending {
		if adm.state != admissionPlaced {
			continue
		}
		switch {
		case holds(inCheckpoint, adm.hash):
			adm.state = admissionCovered
		case holds(kept, adm.hash):
		default:
			adm.state = admissionQueued
		}
	}
}

// decideCovered rejects proven refusals and reports unknown coverage with
// ErrDurabilityUnknown and ErrOutcomeHidden; a successor cannot settle it.
func (a *Adapter) decideCovered(refused, unknown []*spaceAdmission) {
	const reason = "refused before the installed checkpoint"
	if len(refused) != 0 {
		a.decide(refused, world.Rejected, reason, errors.Wrap(sobject.ErrRejectedOp, reason))
	}
	for _, adm := range unknown {
		a.fail(adm, durabilityUnknown(adm.receipt.OpHash(), world.ErrOutcomeHidden))
	}
}

// queued handles queue failures and acknowledgments, and decides an operation
// a proven installed checkpoint already covers. In legacy mode it also
// consumes early replay refusals that lacked a matching admission.
func (a *Adapter) queued(adm *spaceAdmission, localID string, pos *sobject.SOOperationPosition, err error) {
	// Keep unknown writes available for settlement rather than resending them.
	switch {
	case errors.Is(err, errBaseUnsigned):
		a.decide([]*spaceAdmission{adm}, world.Superseded, reasonExpectedWrite, nil)
		return
	case errors.Is(err, sobject.ErrStateWriteUnknown):
		if !sobject.ReloadFenceOff {
			a.stopWrites()
		}
		a.fail(adm, durabilityUnknown(adm.receipt.OpHash(), err))
		return
	case errors.Is(err, world.ErrEngineRetired):
		a.fail(adm, err)
		return
	case errors.Is(err, sobject.ErrMountReloading):
		// Nothing was written, and the mount writes nothing more.
		a.stopWrites()
		a.fail(adm, errors.Wrap(world.ErrEngineRetired, err.Error()))
		return
	case err != nil:
		a.fail(adm, err)
		return
	case adm.op == nil:
		a.decide([]*spaceAdmission{adm}, world.Accepted, "", nil)
		return
	}

	// Reconcile the durable position against replay refusal and checkpoint proof.
	var refused *spacePosition
	var outcome coverOutcome
	var installedHead bool
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Retain coverage proof and complete legacy callback attribution.
		adm.pos = pos
		if a.legacyReplayMatcher {
			adm.localID = localID
			key := unmatchedRefusalKey(localID, pos.GetOpHash())
			if replayed, ok := a.unmatched[key]; ok && (refusalHandoffOff || replayed.op.Inner.GetLocalId() == localID) {
				delete(a.unmatched, key)
				refused = &replayed
				return
			}
		}

		// Check the installed checkpoint only when replay did not answer.
		outcome = a.coverLocked(adm)
		installedHead = outcome == coverApplied && a.acceptHeight >= a.cover.height
	})
	switch {
	case refused != nil:
		a.rejectAdmission(adm, refused.reason)
	case outcome == coverRefused:
		a.decideCovered([]*spaceAdmission{adm}, nil)
	case outcome == coverUnknown:
		a.decideCovered(nil, []*spaceAdmission{adm})
	case outcome == coverApplied && installedHead:
		a.decide([]*spaceAdmission{adm}, world.Accepted, "", nil)
	}
}

// Acknowledge waits for a queued acknowledgment after all admitted operations.
func (a *Adapter) Acknowledge(ctx context.Context) error {
	_, err := a.enqueue(nil).WaitDecided(ctx)
	return err
}

// stopWrites fences the serial queue and write gate after an unknown outcome
// or mount reload. Later writes stay unsent until the mount ends, so settlement
// cannot include writes after the unknown. ReloadFenceOff retains the previous
// behavior, allowing those later writes until reload ends the run.
func (a *Adapter) stopWrites() {
	a.queue.stop(world.ErrEngineRetired)
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !a.reloading {
			a.reloading = true
			broadcast()
		}
	})
}

// fail ends adm with err: its operation never entered the order.
func (a *Adapter) fail(adm *spaceAdmission, err error) {
	a.decide([]*spaceAdmission{adm}, world.Undecided, "", err)
}

// decide removes undecided admissions, recomputes the writer head and wakes
// their waiters after recording their outcomes at the installed head.
func (a *Adapter) decide(adms []*spaceAdmission, decision world.Decision, reason string, err error) {
	// Record the decisions and recompute the writer head.
	var done []*spaceAdmission
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, adm := range adms {
			if adm.state == admissionDecided {
				continue
			}
			adm.state, adm.decision, adm.reason, adm.err, adm.head = admissionDecided, decision, reason, err, a.head
			a.pending = slices.DeleteFunc(a.pending, func(o *spaceAdmission) bool { return o == adm })
			a.settleDecidedLocked(adm)
			done = append(done, adm)
		}
		a.writer = a.writerHeadLocked()
	})

	// Wake the waiters.
	for _, adm := range done {
		close(adm.done)
	}
}

// writerHeadLocked returns the last pending candidate's head or the installed
// head. The caller holds bcast.
func (a *Adapter) writerHeadLocked() world.Head {
	if c := a.writerAdmissionLocked().getCandidate(); c != nil {
		return world.Head{Root: c.Root, Seqno: c.Seqno}
	}
	return a.head
}

// writerAdmissionLocked finds the last pending candidate, or nil for the
// installed head. The caller holds bcast.
func (a *Adapter) writerAdmissionLocked() *spaceAdmission {
	for _, adm := range slices.Backward(a.pending) {
		if adm.getCandidate() != nil {
			return adm
		}
	}
	return nil
}

// installHead publishes the last replay position, holds its retained roots,
// notifies head watchers, then decides the admissions it proves applied.
func (a *Adapter) installHead(ctx context.Context) error {
	// Install the head.
	state, seqno := a.replay.head()
	height, positions := a.replay.height, len(a.replay.positions)
	head := world.Head{Root: state.GetHeadRef().GetRootRef().Clone(), Seqno: seqno, Durable: true}
	var watchers []*headWatch
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Advance the replay position even when refusal leaves the World unchanged.
		moved := a.headHeight != height || a.headPositions != positions
		a.headHeight, a.headPositions = height, positions
		changed := !state.EqualVT(a.headState) || a.head.Seqno != seqno
		if changed {
			a.head, a.headState = head, state
			a.writer = a.writerHeadLocked()
		}
		if changed || moved {
			broadcast()
		}
		watchers = slices.Clone(a.watchers)
	})

	// Retry retained-root holds until publication succeeds.
	if err := a.holdRetained(ctx, state.GetRetainedRoots()); err != nil {
		return err
	}

	// Notify watchers before deciding candidates, without repeating heads.
	if !head.Root.GetEmpty() {
		a.watchMtx.Lock()
		if !head.Root.EqualsRef(a.reported.Root) || head.Seqno != a.reported.Seqno {
			for _, w := range watchers {
				w.fn(head)
			}
			a.reported = head
		}
		a.watchMtx.Unlock()
	}

	// Collect candidates with the finished height atomically so queued cannot
	// miss both this collection and its immediate-acceptance check.
	var placed []*spaceAdmission
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.acceptHeight = height
		for _, adm := range a.pending {
			if adm.state == admissionPlaced || adm.state == admissionCovered {
				placed = append(placed, adm)
			}
		}
	})
	if len(placed) != 0 {
		a.decide(placed, world.Accepted, "", nil)
	}
	return nil
}

// moveRoots stores reachable kept pages and swaps roots from their held values.
func (a *Adapter) moveRoots(ctx context.Context, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	// Flush additional reachable pages before locking for the root swap.
	var reach []*block.BlockRef
	inBatch := func(ref *block.BlockRef) bool {
		return slices.ContainsFunc(entries, func(e *block.PutBatchEntry) bool { return refEqual(e.Ref, ref) })
	}
	for _, e := range entries {
		reach = append(reach, slices.DeleteFunc(slices.Clone(e.Refs), inBatch)...)
	}
	for _, root := range roots {
		if !inBatch(root.Ref) {
			reach = append(reach, root.Ref)
		}
	}
	release, err := a.lockRoots(ctx, reach...)
	if err != nil {
		return err
	}
	defer release()
	return a.moveRootsLocked(ctx, entries, roots...)
}

// lockRoots flushes reach outside rootMtx, then locks only for the root swap.
// Release unlocks before ending the flush. Any additional reachable pages are
// stored by the swap itself. A transient replay skips the flush. A root no
// source serves fails it with block.ErrUnavailable.
func (a *Adapter) lockRoots(ctx context.Context, reach ...*block.BlockRef) (func(), error) {
	release, unavailable, err := a.lockRootsServed(ctx, reach)
	if err == nil && len(unavailable) != 0 {
		release()
		return nil, unavailableRootErr(unavailable)
	}
	return release, err
}

// lockRootsServed is lockRoots returning the roots no source served, which
// the flush pinned nowhere, instead of failing.
func (a *Adapter) lockRootsServed(ctx context.Context, reach []*block.BlockRef) (func(), []*block.BlockRef, error) {
	// Store the pages outside the lock.
	endFlush := func() {}
	var unavailable []*block.BlockRef
	if !a.opts.transient && len(reach) != 0 {
		var err error
		if endFlush, unavailable, err = a.pages.Flush(ctx, reach); err != nil {
			return nil, nil, err
		}
	}

	// Take the lock for the swap.
	unlock, err := a.rootMtx.Lock(ctx)
	if err != nil {
		endFlush()
		return nil, nil, err
	}
	return func() {
		unlock()
		endFlush()
	}, unavailable, nil
}

// moveRootsLocked swaps roots through the kept pages. The caller holds rootMtx.
func (a *Adapter) moveRootsLocked(ctx context.Context, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	return a.moveRootsOn(ctx, a.pages, entries, roots...)
}

// moveRootsOn swaps changed held roots under the caller's rootMtx hold.
func (a *Adapter) moveRootsOn(ctx context.Context, store block.RootOps, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	// A transient replay holds nothing.
	if a.opts.transient {
		return nil
	}

	// Move only the roots that change, from what they hold.
	moves := make([]block.NamedRoot, 0, len(roots))
	for _, root := range roots {
		prev := a.held[root.Name]
		if refEqual(prev, root.Ref) {
			continue
		}
		root.Previous = prev
		moves = append(moves, root)
	}
	if len(moves) == 0 && len(entries) == 0 {
		return nil
	}
	if err := store.SetRoots(ctx, entries, moves); err != nil {
		return err
	}

	// Record what they hold.
	for _, root := range moves {
		if root.Ref.GetEmpty() {
			delete(a.held, root.Name)
			continue
		}
		a.held[root.Name] = root.Ref.Clone()
	}
	return nil
}

// loadRoots reads the roots an earlier run held.
func (a *Adapter) loadRoots(ctx context.Context) error {
	// Take the root lock.
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Read each root, and the payloads and Worlds their blocks reference.
	if err := a.readHeldRoots(ctx, heldRootNames); err != nil {
		return err
	}
	if err := a.readRootSets(ctx); err != nil {
		return err
	}
	return a.readRetained(ctx)
}

// readHeldRoots loads names into held under the caller's rootMtx hold.
func (a *Adapter) readHeldRoots(ctx context.Context, names []string) error {
	for _, name := range names {
		ref, err := a.pages.GetRoot(ctx, name)
		if err != nil {
			return err
		}
		if ref.GetEmpty() {
			delete(a.held, name)
			continue
		}
		a.held[name] = ref
	}
	return nil
}

// readRootSets loads histories and payloads under the caller's rootMtx hold.
func (a *Adapter) readRootSets(ctx context.Context) error {
	for _, h := range []*rootSet{&a.cursorWorlds, &a.pendingWorlds, &a.checkpointWorlds, &a.imported, &a.payloads} {
		if err := a.readRootSet(ctx, h); err != nil {
			return err
		}
	}
	return nil
}

// ownRoots belong exclusively to the Adapter. A competing move makes its held
// view stale and must fail rather than overwrite that writer.
var ownRoots = []string{
	RootReplayBase,
	RootCheckpoint,
	RootReplayCursor,
	RootReplayPayloads,
	RootPending,
}

// retryMovedLocked retries ErrRootMoved only for an import append that still
// reaches the held import root within maxRootLinks. Every ownRoot must match.
// HoldImport only appends, so the retry keeps all previously held roots and reads
// the added ones before release decisions. Any other move could have released a
// needed root and must fail. The caller holds rootMtx.
func (a *Adapter) retryMovedLocked(ctx context.Context, move func() error) error {
	// Move, and return unless another writer moved a root first.
	err := move()
	if !errors.Is(err, block.ErrRootMoved) {
		return err
	}

	// Fail unless the import root is the only root that moved.
	for _, name := range ownRoots {
		ref, rerr := a.pages.GetRoot(ctx, name)
		if rerr != nil {
			return rerr
		}
		if !refEqual(ref, a.held[name]) {
			return err
		}
	}
	imported, rerr := a.pages.GetRoot(ctx, RootImport)
	if rerr != nil {
		return rerr
	}
	appended, rerr := a.appendsTo(ctx, imported, a.held[RootImport])
	if rerr != nil {
		return rerr
	}
	if !appended {
		return err
	}

	// Read the import root again and move once more.
	if err := a.readImportLocked(ctx); err != nil {
		return err
	}
	return move()
}

// readImportLocked refreshes the import chain after HoldImport appends.
// Cost: one root read per trim and one chain read after an append.
// The caller holds rootMtx.
func (a *Adapter) readImportLocked(ctx context.Context) error {
	// Leave the roots in memory when the stored root has not moved.
	ref, err := a.pages.GetRoot(ctx, RootImport)
	if err != nil {
		return err
	}
	if refEqual(ref, a.held[RootImport]) {
		return nil
	}

	// Read the moved chain.
	if err := a.readHeldRoots(ctx, []string{RootImport}); err != nil {
		return err
	}
	return a.readRootSet(ctx, &a.imported)
}

// appendsTo proves ref differs from and reaches held within maxRootLinks.
// The caller holds rootMtx.
func (a *Adapter) appendsTo(ctx context.Context, ref, held *block.BlockRef) (bool, error) {
	if refEqual(ref, held) {
		return false, nil
	}
	for range maxRootLinks {
		if ref.GetEmpty() {
			return false, nil
		}
		data, err := a.readRootBlock(ctx, RootImport, ref)
		if err != nil {
			return false, err
		}
		msg := &HeldRoots{}
		if err := msg.UnmarshalVT(data); err != nil {
			return false, err
		}
		ref = msg.GetPrev()
		if refEqual(ref, held) {
			return true, nil
		}
	}
	return false, nil
}

// addPayloads holds payloads before queueing or after remote replay. Local
// writers require an existing hold because a released stage may have left its
// tree available to collection. Root swaps store only the writer's pages.
func (a *Adapter) addPayloads(ctx context.Context, extra []*block.BlockRef, requireHeld bool) error {
	release, err := a.lockRoots(ctx)
	if err != nil {
		return err
	}
	defer release()
	return a.appendRoots(ctx, &a.payloads, extra, requireHeld)
}

// sweep drops the kept pages no live World reaches: the replay base's, the
// live Worlds' in memory, and their payloads and retained roots.
func (a *Adapter) sweep() {
	// Mark the pages kept so far, then gather the live Worlds: a page kept
	// after the mark may belong to a candidate admitted meanwhile.
	mark := a.pages.SweepMark()
	l := a.gatherLive()

	// Drop the pages none of them reaches.
	base := a.replay.base.GetHeadRef().GetRootRef()
	a.pages.Sweep(slices.Concat([]*block.BlockRef{base}, l.memory, l.candidates, l.payloads, l.retained), mark)
}

// buildLookupOp resolves static operation types, then enabled bus handlers.
func (a *Adapter) buildLookupOp() world.LookupOp {
	b := a.opts.Bus
	if a.opts.DisableLookup {
		b = nil
	}
	return a.opts.OpTypes.LookupOn(b, a.le, a.opts.EngineID)
}

// newCursor shares the decoded cache and head transform with linked files and
// stored-page accounting. The transform must be set before reading files.
func (a *Adapter) newCursor(ctx context.Context, store block.StoreOps, head *bucket.ObjectRef) (*bucket_lookup.Cursor, error) {
	// Build the cursor.
	cursor, err := a.cursors.Open(ctx, store, a.bucketID, head)
	if err != nil {
		return nil, err
	}

	// Give the linked files and the stored page count the Space's transform.
	if files := a.so.GetBlockStore().GetFileSegments(); files != nil {
		files.SetTransformer(cursor.GetTransformer())
	}
	return cursor, nil
}

// refsEntry encodes msg as a block whose outgoing refs are refs.
func refsEntry(msg interface{ MarshalVT() ([]byte, error) }, refs []*block.BlockRef) (*block.PutBatchEntry, error) {
	// Encode the message.
	data, err := msg.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Address it with its refs.
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, err
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}, nil
}

// refEqual reports whether a and b reference the same block, or both none.
func refEqual(a, b *block.BlockRef) bool {
	if a.GetEmpty() || b.GetEmpty() {
		return a.GetEmpty() == b.GetEmpty()
	}
	return a.EqualsRef(b)
}

// _ verifies the Adapter's body and authority contracts.
var (
	_ world.Authority = (*Adapter)(nil)
	_ sobject.Body    = (*Adapter)(nil)
)
