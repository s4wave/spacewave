package space_world

import (
	"bytes"
	"context"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// ReplayIdentityEnv selects the legacy replay matcher when set to "off"
// before opening the adapter.
const ReplayIdentityEnv = "SPACEWAVE_WORLD_REPLAY_IDENTITY"

// RefusalHandoffEnv selects the local-ID-only refusal cache in legacy replay
// mode when set to "off" before process start.
const RefusalHandoffEnv = "SPACEWAVE_WORLD_REFUSAL_HANDOFF"

// refusalHandoffOff keeps the previous refusal path selectable at startup.
var refusalHandoffOff = os.Getenv(RefusalHandoffEnv) == "off"

// Adapter serves a Space whose body is a World: it is the sobject.Body the
// SharedObject replays its agreed order into, the world.Authority of the one
// World engine that serves the Space, and the holder of the named roots
// replay needs on the SharedObject's block store.
//
// A local write is a turn of the engine. Its candidate is queued as an
// operation and decided once the driver applies it: Accepted when the
// operation applied at its position, Superseded when replay refused it as a
// conflict with an operation the writer did not build on, and Rejected for any
// other refusal. A decided candidate is durable at once: the SharedObject
// holds its operation durably, and replay from the checkpoint rebuilds the
// head from the operation log after a restart, so the head needs no stored
// pages to survive. The next writer's turn begins on the admitted candidate
// before it is decided, so writes pipeline.
//
// The adapter keeps the pages of every World it computes in a PendingStore and
// stores a World only where a named root moves onto it: the checkpoint, the
// replay base and the replay cursor. Pages written and replaced between those
// Worlds never reach the Volume. A backfill or fence copy stores the Worlds it
// holds and leaves their pages kept. An operation's payloads, which no World
// need reach, are held under the replay payloads root before the operation is
// queued, so the operation log never references a payload a restart cannot
// read. Replay holds another device's payloads the same way as it applies the
// operation, so a successor that owns the Space after a handover holds every
// payload above the checkpoint once its peer is gone.
//
// Invariants:
//   - The adopted checkpoint's World is held, by replay-base or checkpoint, at
//     every point, so a restart always finds the World replay starts from.
//   - The replay cursor record references a World only after replay-cursor
//     holds it.
//   - The writer head is the last pending candidate, or the installed head
//     when none is pending, and a candidate's base is the writer head when it
//     was admitted.
//   - The installed head never loses an operation an earlier installed head
//     applied, until replay places that operation again or refuses it.
//   - A candidate is decided only after every head watcher saw the head that
//     holds it.
//   - Every stored page a live World references is held, by a named root whose
//     World reaches it through child lists. The live Worlds are the ones
//     gatherLive returns, which the sweep and the trim share: the positions,
//     the restored positions, the installed and writer heads, the candidates
//     and the Worlds a reader of the served engine still reads. The replay
//     base or a history root releases a World only in the root move that
//     stores and holds the live Worlds in memory in its place, so a trim
//     releases only Worlds that no held or live World reaches.
type Adapter struct {
	// le is the logger.
	le *logrus.Entry
	// opened closes once Execute claimed the Space and read what an earlier
	// run held. The body methods wait for it, since the driver steps the body
	// as soon as it attaches.
	opened chan struct{}
	// so is the Space's SharedObject.
	so sobject.SharedObject
	// opts are the adapter's options.
	opts AdapterOptions
	// lookupOp resolves World operations: the built-in types, then the bus.
	lookupOp world.LookupOp
	// payloadVersionsMtx guards payloadVersions: the driver goroutine and
	// the writers' Admit both read it.
	payloadVersionsMtx sync.Mutex
	// payloadVersions caches OpTypes.payloadVersion by operation type.
	payloadVersions map[string]uint32
	// bucketID is the SharedObject block store's bucket.
	bucketID string
	// decoded is the decoded block cache every World cursor shares.
	decoded *block.DecodedBlockCache
	// pages keeps the World pages in front of so's block store. The serving
	// engine and the replay engine read and write through it.
	pages *block.PendingStore
	// storedPages reads the kept World pages it counts as they are stored.
	storedPages storedPages
	// queue queues admitted operations, in admission order.
	queue *opQueue
	// backfill keeps a local copy of the World while this device chose it.
	backfill *spaceBackfill
	// replay holds the positions above the installed checkpoint. The driver
	// goroutine owns it.
	replay *spaceReplay
	// missing holds the operations an installed head applied that a reorder
	// put after the positions replay has applied again. The installed head
	// stays until missing is empty. The driver goroutine owns missing.
	missing map[string]struct{}
	// turnMtx is held from Begin to the turn's Release, so one local writer
	// proposes at a time.
	turnMtx csync.Mutex
	// rootMtx serializes the named root swaps. lockRoots takes it after it
	// stored the pages the swap reaches, so it is held only for the swap: a
	// local writer that holds a root never waits on a store of the driver's
	// pages.
	rootMtx csync.Mutex
	// held are the named roots this adapter holds, by name. Guarded by
	// rootMtx.
	held map[string]*block.BlockRef
	// payloads are the payload roots the replay payloads root holds.
	payloads rootSet
	// stateHold is what the payload hold knows of the last order Hold saw,
	// nil before the first. The driver goroutine uses it.
	stateHold *payloadState
	// resetPlanned, when set, runs after resetPayloads plans a drop and
	// before it takes rootMtx for the move, so a test can admit a writer
	// there. Nil outside tests.
	resetPlanned func()
	// retainedHeld are the retained roots the retained root holds. Guarded
	// by rootMtx.
	retainedHeld []*RetainedRoot
	// cursorWorlds, pendingWorlds and checkpointWorlds are the Worlds the
	// replay cursor, pending and checkpoint roots hold, each until a
	// checkpoint covers them.
	cursorWorlds, pendingWorlds, checkpointWorlds rootSet
	// imported are the roots the import root holds for writers outside the
	// Adapter, each until a trim holds it under the replay base or finds the
	// payloads root holding it (see HoldImport).
	imported rootSet
	// watchMtx serializes the head watchers' callbacks.
	watchMtx sync.Mutex
	// reported is the last head reported to the watchers. Guarded by
	// watchMtx.
	reported world.Head

	// bcast guards the fields below.
	bcast broadcast.Broadcast
	// head is the installed head. Guarded by bcast.
	head world.Head
	// headState is the World state of head. Guarded by bcast.
	headState *InnerState
	// writer is the head the next turn builds on: the last admitted
	// candidate, or head when none waits. Guarded by bcast.
	writer world.Head
	// pending are the admissions not yet decided, in admission order.
	// Guarded by bcast.
	pending []*spaceAdmission
	// fences are the storage fences the driver applied, by hash, with their
	// answers. Guarded by bcast.
	fences map[string]*spaceFence
	// legacyReplayMatcher selects the pre-receipt replay and queue paths.
	legacyReplayMatcher bool
	// expectWrites is whether this device's writes carry an ExpectedWrite. A
	// legacy matcher queues without a receipt, so it has no hash to expect.
	expectWrites bool
	// unmatched holds early refusals only in legacy mode. Guarded by bcast.
	unmatched map[string]spacePosition
	// cover is what the installed checkpoint says about this device's
	// operations, or nil before the first install. Guarded by bcast.
	cover *checkpointCover
	// watchers receive each installed head. Guarded by bcast.
	watchers []*headWatch
	// installed is set once the driver installed the first checkpoint.
	// Guarded by bcast.
	installed bool
	// checkpoint and checkpointSeqno are the installed checkpoint's World and
	// its seqno, which backfill copies. Guarded by bcast.
	checkpoint      *InnerState
	checkpointSeqno uint64
	// headHeight and headPositions place the installed head: the height of
	// its checkpoint and how many positions above it it follows. Guarded by
	// bcast.
	headHeight    uint64
	headPositions int
	// acceptHeight is the checkpoint height of the last head installHead
	// finished: it held the head's retained roots and reported the head to
	// every watcher. A candidate the installed cover places is accepted at
	// once only at or below acceptHeight. Otherwise the install finishing
	// there accepts it. Guarded by bcast.
	acceptHeight uint64
	// replayState is the replay state the body driver last reached, opening
	// before its first step. It alone decides whether the Space takes writes
	// (gateWrite). Guarded by bcast.
	replayState *sobject.SOReplayState
	// handlerWait is replay's wait for a handler, or nil. Owned by the body
	// driver's goroutine, which calls Apply and Install.
	handlerWait *handlerWait
	// engine is the serving engine once built, until it retires. Guarded by
	// bcast.
	engine *Engine
	// storing holds the Worlds each background store keeps live, such as a
	// checkpoint's, a cursor save's or a trim's, under the store's own token
	// until it returns. The sweep and the trim keep them live, and Execute
	// waits for it to empty before it releases the Space. Guarded by bcast.
	storing map[uint64][]*block.BlockRef
	// storingNext is the next background store's token. Guarded by bcast.
	storingNext uint64
	// stopped reports that Execute's run ended, so no background store
	// starts. Guarded by bcast.
	stopped bool
	// reloading reports that a write's outcome went unknown or the mount
	// refused a write because it will reload. It is terminal: gateWrite
	// holds every later write until the engine stops serving, then refuses
	// it, and the queue sends nothing more, so no write commits on this
	// mount between the unknown and its settle. Guarded by bcast.
	reloading bool
	// settles settles the writes of this device whose outcome went unknown,
	// shared with the other adapters of the Space World.
	settles *settleSet
	// runCtx ends every background store when Execute's run ends.
	runCtx context.Context
	// cursorSaving reports that a cursor save runs in the background.
	cursorSaving atomic.Bool
	// trimMtx runs the background trims one at a time.
	trimMtx csync.Mutex
	// trimSeq numbers the trims the driver gathers. A trim older than the
	// newest is skipped.
	trimSeq atomic.Uint64
	// readers is the engine whose readers the sweep and the history trim keep
	// live, from when it serves until it closes, past its retirement. Guarded
	// by bcast.
	readers worldReaders
	// holders counts the AcquireEngine holds on engine. Guarded by bcast.
	holders int
	// ownership is the ownership last derived, for config head ownershipHead.
	// Guarded by bcast.
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

	// Front the SharedObject's block store with the kept pages and the shared
	// decoded block cache.
	store := so.GetBlockStore()
	decoded := store.GetDecodedBlockCache()
	if decoded == nil {
		decoded = block.NewDecodedBlockCache()
	}
	a := &Adapter{
		le:                  le,
		opened:              make(chan struct{}),
		so:                  so,
		opts:                opts,
		bucketID:            store.GetID(),
		decoded:             decoded,
		pages:               block.NewPendingStore(store, opts.MaxPendingBytes),
		queue:               &opQueue{so: so},
		missing:             make(map[string]struct{}),
		legacyReplayMatcher: os.Getenv(ReplayIdentityEnv) == "off",
		expectWrites:        os.Getenv(ReplayIdentityEnv) != "off" && os.Getenv(ExpectWriteEnv) != "off",
		fences:              make(map[string]*spaceFence),
		held:                make(map[string]*block.BlockRef),

		cursorWorlds:     rootSet{name: RootReplayCursor},
		pendingWorlds:    rootSet{name: RootPending},
		checkpointWorlds: rootSet{name: RootCheckpoint},
		imported:         rootSet{name: RootImport},
		payloads:         rootSet{name: RootReplayPayloads},
		payloadVersions:  make(map[string]uint32),
		replayState:      newReplayOpening(),
		settles:          settles,
	}
	a.pages.SetKeepStaged(keepStaged)
	a.lookupOp = a.buildLookupOp()
	a.replay = &spaceReplay{a: a}
	a.backfill = newSpaceBackfill(a)

	// A transient replay claims nothing and reads no roots, so its driver
	// steps it at once, holding what it stores in one epoch its caller ends.
	if opts.transient {
		a.replay.stage = newReplayStage(a.pages)
		close(a.opened)
	}
	return a, nil
}

// Execute serves the Space World until ctx ends: it claims the Space as the
// SharedObject's one body, reads the held roots and the saved cursor, runs the
// operation queue and the body driver, initializes an empty World as an
// owner, and builds the engine at the installed head.
func (a *Adapter) Execute(ctx context.Context) error {
	// Claim the Space. SetBody waits until the previous adapter of the Space
	// released it after it stopped, so this one reads the roots only once no
	// other adapter moves them. The release runs last, after the engine
	// retired.
	stopBody, detach, err := a.so.SetBody(ctx, a)
	if err != nil {
		return err
	}
	defer detach()
	runCtx, stopRun := context.WithCancel(ctx)
	a.runCtx = runCtx

	// Stop body calls before releasing replay resources on every exit,
	// including startup failure while the driver waits for opened. Keep the
	// claim until this cleanup returns. detach is the last defer.
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

	// Take the writes an earlier mount left unknown before the driver's
	// first step, so replay matches each one.
	adopted := a.adoptSettles()
	close(a.opened)

	// Run the queue and the storage reclaim, and settle the adopted writes
	// the loaded state lacks.
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return a.queue.execute(ctx, a.queued) })
	g.Go(func() error { return a.executeStorageReclaim(ctx) })
	if len(adopted) != 0 {
		g.Go(func() error { return a.settleAdopted(ctx, adopted) })
	}

	// Serve the engine once the World is initialized.
	g.Go(func() error {
		// Build and publish the engine.
		engine, err := a.serve(ctx)
		if err != nil {
			return err
		}

		// End the admissions when the run ends, stop serving the engine, and
		// retire it: the transactions open on it end as superseded, and their
		// callers run again on the next adapter.
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

// startBackground starts a background store that keeps roots live, under its
// own token, so a sweep keeps their pages for the store, and Execute waits for
// the store until the returned done is called. A second call does nothing. It
// reports false, and starts nothing, once Execute's run ended.
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

// backgroundContext returns ctx, also ended when Execute's run ends, for a
// background store. Its root moves commit ordered, without waiting on a disk
// sync under rootMtx: a crash that drops one only repeats a trim, a hold or a
// cursor save, and the next durable commit flushes it in order.
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

// waitOpened waits until Execute claimed the Space and read what an earlier
// run held.
func (a *Adapter) waitOpened(ctx context.Context) error {
	select {
	case <-a.opened:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// endAdmissions ends every admission still undecided at unmount. One still
// waiting in the queue never reached the SharedObject, so it is superseded:
// its writer runs again on the next head. Any other may be durable, such as
// one inside QueueOperation, so its durability is unknown. The next open
// replays its operation if the SharedObject holds it, and settles it for
// WaitSettled. An adopted write stays unsettled for the next adapter.
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
		a.fail(adm, durabilityUnknown(adm, errAdapterStopped))
	}
}

// WaitEngine waits for the engine serving the Space World.
func (a *Adapter) WaitEngine(ctx context.Context) (*Engine, error) {
	var engine *Engine
	err := a.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		engine = a.engine
		return engine != nil, nil
	})
	return engine, err
}

// AcquireEngine waits for the serving engine and holds it open until release
// is called. The adapter retires the engine before it closes it, and waits
// for every hold first, so a holder never sees it close. A holder releases
// once WaitEngineReplaced returns, and writes through another reference,
// such as a lookup: a write on a reloading adapter waits for every hold to
// end (gateWrite), so a write under the caller's own hold waits until its
// ctx ends.
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

// serve waits for an initialized World, queuing its initialization once as
// an owner, then builds the engine at the installed head and publishes it.
func (a *Adapter) serve(ctx context.Context) (*Engine, error) {
	// Read the order replay must reach before the engine serves, so readers
	// never see a World behind the one the Space held at open.
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	target, err := sobject.ReadOrder(ctx, snap)
	if err != nil {
		return nil, err
	}

	// Wait for an initialized head at or past that order, or at an
	// operation replay waits at, so reads go on while the device lacks its
	// handler. Writes wait for the catch up.
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

// Begin waits for the local writer turn. A participant whose role cannot
// write, or an owner handing over, gets tx.ErrNotWrite, since replay would
// refuse every operation it writes. While replay opens, Begin waits for it
// until ctx ends. While replay waits, Begin returns a *sobject.WriteWait. The
// turn's Base is the writer head, not durable while a candidate admitted
// before it waits.
func (a *Adapter) Begin(ctx context.Context) (world.Turn, error) {
	// Wait while replay opens, and refuse a write while it waits or
	// reloads. The admission gates again, as replay may stop at a wait
	// before it. This gate comes first, since a reloading mount ends and
	// then fails the role check's state read with its own error, which no
	// retry on the successor would follow.
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

// checkWrites returns tx.ErrNotWrite when this participant cannot write
// operations or is an owner handing over.
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

// ownershipOf returns the ownership of snap's config, derived once per config
// head.
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

// Root returns the encoded InnerState of the World after the first n
// positions, and the store that moves the checkpoint root onto it beside
// every World returned since the last install. Until store returns, the World
// is live, so a sweep keeps its pages and a trim holds it. store flushes its
// kept pages in slices before it takes the root lock for the swap, so the
// driver may run it beside its apply loop.
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

		// Hold it beside every World returned since the last install,
		// unless an install already made it the replay base, which holds
		// it.
		a.countStoredPages(ctx, root)
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

// Apply computes the World after op at index. An operation of this device
// whose candidate was built on the World before index takes the candidate's
// root without running op again. Any other runs on the replay engine. When the
// positions hold every operation the installed head applied, the head after
// the last one is installed, and the candidates it holds are decided.
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

	// Find this device's candidate for the operation, and take its World
	// when it was built on the World before index.
	adm := a.match(op)
	prev, _ := r.stateBefore(index)
	var adopt *spacePosition
	expected := adm.getExpected()
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

	// Compute the position. At an operation whose type no handler
	// resolves, return the wait for its handler, so the driver steps
	// again with no lock held once the handler registers. At a block or
	// plugin this device lacks, return the wait for that block or plugin,
	// which the driver retries after a backoff. At a body this build
	// cannot read, return the wait for a build that reads the body, which
	// only a restart or the owner's skip of the operation ends. A
	// transient replay has no driver to wait in.
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

	// Hold the payloads of another device's operation, as its writer did
	// before queueing it, so the device that later owns the Space holds every
	// payload above the checkpoint without the peer that wrote it.
	if len(pos.payloads) != 0 {
		if err := a.addPayloads(ctx, pos.payloads, false); err != nil {
			return "", err
		}
	}

	// Decide or place the candidate, and install the head when it lost
	// nothing.
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

// Install moves the replay base to the World of ev's checkpoint and trims
// the histories, keeping the positions after the covered ones when the World
// after them is the checkpoint's. It decides the admissions the checkpoint
// covers from the operations held, as installCoverLocked does, and updates
// the missing set from ev's operations above it, as noteMissing does.
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

	// Keep the positions after the covered ones when the replay reached the
	// checkpoint's World there.
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

	// Release the payloads of the operations it covers, which replay reads
	// no more and whose effect the kept World holds. Operations a missed
	// install leaves above it keep theirs, as the state still holds them.
	if paid {
		if err := a.resetPayloads(ctx, true); err != nil {
			return false, err
		}
	}

	// Hold the checkpoint's World as the replay base, and trim the Worlds it
	// covers from the histories, in one root move.
	if err := a.trimInBackground(ctx); err != nil {
		return false, err
	}

	// Record the install, and the admissions its coverage decides, then drop
	// the pages no live World reaches.
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

// releases reports whether the trim releases root from a history: a World a
// history held at the gather that is the base or not live. A World a history
// gained since, such as a background store's, stays.
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

// trimInBackground gathers a trim on the driver goroutine and runs it beside
// the driver's apply loop, one trim at a time, so an install never waits for
// the store of the Worlds it holds. A plan older than the newest one gathered
// is skipped, since its base would move the replay base back, and a failed
// trim tries again with backoff while its plan is the newest, so an adopted
// checkpoint's World is held under replay-base soon after its install. Until
// the trim's root move, the old base stays held under replay-base and the
// installed checkpoint's World under the checkpoint root.
func (a *Adapter) trimInBackground(ctx context.Context) error {
	// Gather the plan on the driver goroutine.
	plan, err := a.gatherTrim(ctx)
	if err != nil {
		return err
	}

	// Run it beside the apply loop, keeping the Worlds it holds and its
	// base live until it returns, so a sweep keeps their pages for its
	// flush. A run that ended trims nothing more. The next mount trims.
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

// runTrim runs the trim plan gathered: it moves the replay base onto the
// plan's base World and releases from the cursor, pending and checkpoint
// histories the base World, which its own root holds, and every World the
// plan held that is not live. A World Root returned past the adopted
// checkpoint stays held while a later position still is that World, since
// Root stored its pages and dropped them from memory.
//
// A live World kept in memory may reference stored pages that only the old base
// or an earlier history World holds, so when the base moves or any history
// releases a World, the live Worlds are stored and held in their place: the
// Worlds in memory under the cursor root and the candidates under the pending
// root. The Worlds whose pages left memory stay where a history holds them. The
// base, the holds and every release are one root move, so the trim releases
// only Worlds that no held or live World reaches through its child lists, and a
// failed move releases nothing. The same move releases each imported root the
// Adapter now holds itself (see HoldImport).
//
// Cost per install: one pass over the live Worlds, keyed by ref, and one
// root move that stores the kept pages of the live Worlds.
func (a *Adapter) runTrim(ctx context.Context, p *trimPlan) error {
	// Take the root lock, keeping what the plan does not release.
	keep := func(root *block.BlockRef) bool { return !p.releases(root) }
	reach := slices.Concat(p.memory, p.candidates, []*block.BlockRef{p.base})
	a.countStoredPages(ctx, reach...)
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

		// Leave the roots alone when the base stays and no history
		// releases a World. An imported root is released only when this
		// move holds it under the replay base or the payloads root holds it
		// now, read under the root lock, so a plan gathered before an import
		// appended a root never releases it.
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

// dropPositions drops the replay positions an install covers and returns
// them: the first covered when kept, as the World after them is the
// checkpoint's, and none when not, as every position is dropped. It updates
// the missing set as noteMissing does from above.
func (a *Adapter) dropPositions(covered int, kept bool, above func(h []byte) *sobject.SOOperationInner) []spacePosition {
	// Keep the positions after the covered ones, and forget the missing
	// operations the checkpoint covers.
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

// noteMissing updates the missing set for an install, given the replay
// positions it drops and above, which returns an operation above the
// checkpoint being installed by its hash, or nil. It notes each dropped
// applied position whose operation above returns, so the installed head
// stays until replay applies it again, and forgets each missing hash above
// lacks.
//
// Invariant: a hash leaves the missing set only when replay applies its
// operation again (Apply) or the operations above the checkpoint being
// installed, read from the same snapshot as that checkpoint, no longer hold
// it. An operation leaves them only when that checkpoint covers it, so the
// head installed from it holds the operation: a forgotten hash cannot hold
// installHead back forever, and a hash whose operation can still replay
// stays until replay applies it, so installHead never runs ahead of it. The
// operations above a later checkpoint do not qualify: that checkpoint may
// cover an operation the one being installed does not, so Install takes
// both from one OrderEvent. A kept install drops no positions and still
// forgets: a reorder may have noted an operation the checkpoint covers.
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

// newAdmission returns the admission of op, with candidate when a local
// writer built a World for it.
func (a *Adapter) newAdmission(candidate world.Candidate, op []byte, payloads []*block.BlockRef, handlers *PayloadHandlers) *spaceAdmission {
	return &spaceAdmission{a: a, candidate: candidate, op: op, payloads: payloads, handlers: handlers, done: make(chan struct{})}
}

// enqueue admits the encoded operation op, which builds no World of its own,
// after this device's admitted operations and queues it. A nil op is an
// acknowledgment.
//
// It bypasses gateWrite, which guards a write chosen from the installed
// head: none of its callers reads the head to choose its operation. The init
// operation applies only to an empty World, the acknowledgment carries no
// operation, and a fence and a root set change no World object, so replay
// decides each alike wherever it stands. Gating them would stall serve's
// init and the storage reclaim behind a wait they cannot affect.
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

// admitWrite admits adm as admit does once gateWrite finds replay caught up,
// in the same hold of bcast, so no wait begins between the check and the
// admission. It returns gateWrite's error without admitting adm.
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

// keepAdmitted keeps the blocks of adm's candidate and holds what its queued
// operation needs, through holdAdmitted.
//
// Invariant: a block of the candidate that the put stores past the bound is
// held by the overflow stage from its write until holdAdmitted returns, and a
// block its writer's buffer drained to the writer's stage before Admit
// (candidate.Staged) is held by that stage until the writer's transaction
// ends, after Admit returns. holdAdmitted then holds the candidate's World
// under the pending root, so no collection between the write and the queued
// operation's land frees either, however long the hold waits on the root
// lock. The overflow stage opens only on a put past the bound, so a candidate
// that fits in memory pays no stage.
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
	return a.holdAdmitted(ctx, adm, overflow.Stored())
}

// holdAdmitted holds what adm's queued operation needs once it is durable:
// its payloads under the replay payloads root, and its candidate's World under
// the pending root when its put stored a block past the bound, its writer
// drained blocks to its stage, or the kept pages outgrew memory, so the pages
// its writer's stage or the overflow stage held stay held after they end.
// Each hold is one swap that stores the writer's own pages, and waits on the
// root lock only for another swap, never for the driver's store of its pages.
func (a *Adapter) holdAdmitted(ctx context.Context, adm *spaceAdmission, stored bool) error {
	// Hold the payloads before the operation that references them is
	// queued.
	if len(adm.payloads) != 0 {
		if err := a.addPayloads(ctx, adm.payloads, true); err != nil {
			return err
		}
	}

	// Hold an overflowed, drained or outgrown candidate beside the earlier
	// ones.
	if (!stored && !adm.candidate.Staged && !a.pages.Full()) || adm.candidate.Root.GetEmpty() {
		return nil
	}
	a.countStoredPages(ctx, adm.candidate.Root)
	release, err := a.lockRoots(ctx)
	if err != nil {
		return err
	}
	defer release()

	// The move does not require the blocks it newly reaches held, as
	// block.NamedRoot.RequireHeld would, because each is already kept by
	// one of three: a kept page this move itself stores through the
	// PendingStore's SetRoots; a page an open stage holds, the overflow
	// stage until this returns or the writer's stage until its transaction
	// ends; or a stored page of a live World, which runTrim holds in the
	// same move that releases any World. Requiring them held would instead
	// refuse a candidate over a boundary block revived from zero until its
	// grace passes, and refuse the rebuilt candidate again.
	return a.addRoot(ctx, &a.pendingWorlds, adm.candidate.Root)
}

// match finds the pending admission of this exact signed operation. Neither
// identical WorldOp bytes nor a reused local ID proves replay attribution.
func (a *Adapter) match(op *sobject.OrderedOperation) *spaceAdmission {
	// Only this device's operations can match its pending admissions.
	if op.Inner.GetPeerId() != a.so.GetPeerID().String() {
		return nil
	}

	// Keep the previous matcher selectable at adapter open.
	if a.legacyReplayMatcher {
		return a.matchLegacy(op)
	}

	// Match only the signer's receipt, including when a refusal has no body.
	var out *spaceAdmission
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, adm := range a.pending {
			if adm.receipt.MatchesHash(op.Hash) {
				adm.hash = slices.Clone(op.Hash)
				out = adm
				return
			}
		}
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

// unmatchedRefusalKey indexes cached refusals by the signed hash. The legacy
// handoff keyed by the local ID alone, which can be shared by a fork.
func unmatchedRefusalKey(localID string, hash []byte) string {
	if refusalHandoffOff {
		return localID
	}
	return string(hash)
}

// noteUnmatched is the legacy path that records a refusal Apply did not find, or
// delivers it to the admission whose queue callback filled in its identity
// since Apply looked. The callback can instead arrive after this method and
// consume the refusal it leaves behind.
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

// place records the position of adm's operation. An applied one is decided
// when the installed head holds it. A refused one is decided now: Superseded
// when replay ran it on a World other than its base, so the writer runs its
// change again on the head, and Rejected otherwise.
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

// installCoverLocked installs the cover checkpoint gives peerID's operations,
// proving chain membership from the operations held returns, and applies it to
// every pending admission. It returns the refused and the unknown admissions
// for the caller to decide after the caller releases bcast. An applied one
// becomes covered. It first moves the placed admissions off the replay
// positions the install drops, as reconcilePlacedLocked does, given the
// positions the checkpoint covers and those the replay keeps above it. The
// caller holds bcast.
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

// coverLocked applies the installed checkpoint's cover to adm, a queued
// admission whose position is known, and returns the outcome: an applied one
// becomes covered, for installHead to decide at the installed head, and the
// caller decides a refused or unknown one. The caller holds bcast.
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

// reconcilePlacedLocked moves each placed admission off the replay positions
// an install drops, before the install's cover applies: one placed at a
// position in inCheckpoint, whose World the installed checkpoint's equals, is
// covered. One placed at a position in kept stays placed. Any other lost the
// only evidence that a head holds its operation, so it is queued again, for
// the cover's proof to decide or replay to place again.
//
// Invariant: installHead accepts an admission only when the head it installs
// holds the operation. A placed admission's position is in the replay positions
// that head is built from, or in the missing set that holds the install back
// until replay applies the operation again. A covered one is proven held by an
// installed checkpoint. The caller holds bcast.
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

// decideCovered decides the admissions an installed checkpoint covers without
// replay placing them: refused ones as Rejected, and unknown ones as
// Undecided with ErrDurabilityUnknown, since the SharedObject holds them, and
// world.ErrOutcomeHidden, since no successor can settle them either.
func (a *Adapter) decideCovered(refused, unknown []*spaceAdmission) {
	const reason = "refused before the installed checkpoint"
	if len(refused) != 0 {
		a.decide(refused, world.Rejected, reason, errors.Wrap(sobject.ErrRejectedOp, reason))
	}
	for _, adm := range unknown {
		a.fail(adm, durabilityUnknown(adm, world.ErrOutcomeHidden))
	}
}

// queued handles queue failures and acknowledgments, and decides an operation
// a proven installed checkpoint already covers. In legacy mode it also
// consumes early replay refusals that lacked a matching admission.
func (a *Adapter) queued(adm *spaceAdmission, localID string, pos *sobject.SOOperationPosition, err error) {
	// End a failure or an acknowledgment. A write whose outcome is unknown
	// may hold the operation, so it ends as durability unknown, like one
	// in flight at unmount: the next open replays it if the SharedObject
	// holds it, and the writer does not rebase and send it again.
	switch {
	case errors.Is(err, errBaseUnsigned):
		a.decide([]*spaceAdmission{adm}, world.Superseded, reasonExpectedWrite, nil)
		return
	case errors.Is(err, sobject.ErrStateWriteUnknown):
		if !sobject.ReloadFenceOff {
			a.stopWrites()
		}
		a.fail(adm, durabilityUnknown(adm, err))
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

	// Record the local ID and position, and decide an operation the driver
	// already refused or the installed checkpoint covers. An applied one is
	// decided now when an install that holds the checkpoint finished, and
	// otherwise by the install that finishes there.
	var refused *spacePosition
	var outcome coverOutcome
	var installedHead bool
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Retain the durable position for checkpoint coverage. Only legacy
		// replay learns attribution from the callback.
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

// Acknowledge queues an acknowledgment after every operation this device
// admitted, and returns once the SharedObject holds it.
func (a *Adapter) Acknowledge(ctx context.Context) error {
	// Queue it after the admitted operations, bypassing the write gate as
	// enqueue says, and wait for the SharedObject to hold it.
	_, err := a.enqueue(nil).WaitDecided(ctx)
	return err
}

// stopWrites moves the adapter to Reloading after a write whose outcome is
// unknown or that the mount refused because it will reload.
//
// Invariant: no write of this device commits on this mount between the
// unknown and its settle. The queue is serial, so this runs before it takes
// the next admission. That one and every later one end with
// world.ErrEngineRetired unsent, and gateWrite admits no new one. The
// provider's reload fence refuses the mount's other writers, such as the
// sequencer, the same way. With sobject.ReloadFenceOff the adapter keeps
// writing after an unknown, as before the fence, until the reload ends its
// run, and WaitSettled's head may then hold writes that followed the unknown.
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

// decide ends each undecided admission in adms with decision at the
// installed head, drops it from pending, moves the writer head back past it,
// and wakes its waiters.
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

// writerHeadLocked returns the head the next turn builds on: the last pending
// candidate's World, or the installed head when none is pending. The caller
// holds bcast.
func (a *Adapter) writerHeadLocked() world.Head {
	if c := a.writerAdmissionLocked().getCandidate(); c != nil {
		return world.Head{Root: c.Root, Seqno: c.Seqno}
	}
	return a.head
}

// writerAdmissionLocked returns the last pending admission with a candidate,
// whose World the writer head is, or nil when the writer head is the
// installed head. The caller holds bcast.
func (a *Adapter) writerAdmissionLocked() *spaceAdmission {
	for _, adm := range slices.Backward(a.pending) {
		if adm.getCandidate() != nil {
			return adm
		}
	}
	return nil
}

// installHead installs the World after the last position as the head, moves
// the writer head with it, holds its retained roots, reports it to every head
// watcher, then finishes the install and decides the candidates it holds.
func (a *Adapter) installHead(ctx context.Context) error {
	// Install the head.
	state, seqno := a.replay.head()
	height, positions := a.replay.height, len(a.replay.positions)
	head := world.Head{Root: state.GetHeadRef().GetRootRef().Clone(), Seqno: seqno, Durable: true}
	var watchers []*headWatch
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Move the head when it changed. A refused position moves only the
		// place, which serve waits on.
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

	// Hold the retained roots unless they are held already, so a failed hold
	// is tried again by the next install.
	if err := a.holdRetained(ctx, state.GetRetainedRoots()); err != nil {
		return err
	}

	// Report the head unless the watchers saw it, then decide what it holds,
	// so a candidate is decided only once every watcher saw its head.
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

	// Finish the install and collect the placed candidates in one hold, so a
	// candidate queued places after it either is collected here or sees the
	// finished height and is accepted at once.
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

// moveRoots stores entries and moves each root onto its Ref, with the
// reference this adapter holds under its name as Previous, through the
// PendingStore, which stores the kept pages each moved root reaches.
func (a *Adapter) moveRoots(ctx context.Context, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	// Store what the move reaches beyond its own entries, then swap the
	// roots under the lock.
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

// lockRoots stores the kept pages reach leads to under a PendingStore flush,
// then takes rootMtx, so the lock and the store's writer are held only for
// the root swap the caller makes under it. The release unlocks rootMtx, then
// ends the flush. reach lists the roots the caller's swap may reference. A page
// the swap reaches beyond them is stored in the swap. A transient replay holds
// nothing, so it flushes nothing.
func (a *Adapter) lockRoots(ctx context.Context, reach ...*block.BlockRef) (func(), error) {
	// Store the pages outside the lock.
	endFlush := func() {}
	if !a.opts.transient && len(reach) != 0 {
		var err error
		if endFlush, err = a.pages.Flush(ctx, reach); err != nil {
			return nil, err
		}
	}

	// Take the lock for the swap.
	unlock, err := a.rootMtx.Lock(ctx)
	if err != nil {
		endFlush()
		return nil, err
	}
	return func() {
		unlock()
		endFlush()
	}, nil
}

// moveRootsLocked is moveRoots for a caller holding rootMtx. It moves the
// roots through the kept pages, which store every kept page a moved root
// reaches and drop it from memory.
func (a *Adapter) moveRootsLocked(ctx context.Context, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	return a.moveRootsOn(ctx, a.pages, entries, roots...)
}

// moveRootsOn moves the roots that change on store, from what they hold. The
// caller holds rootMtx.
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

// readHeldRoots reads what each of names holds into held. The caller holds
// rootMtx.
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

// readRootSets reads the roots each history and the payloads root holds. The
// caller holds rootMtx.
func (a *Adapter) readRootSets(ctx context.Context) error {
	for _, h := range []*rootSet{&a.cursorWorlds, &a.pendingWorlds, &a.checkpointWorlds, &a.imported, &a.payloads} {
		if err := a.readRootSet(ctx, h); err != nil {
			return err
		}
	}
	return nil
}

// ownRoots are the roots only the Adapter moves. Another writer moving one
// of them, such as a stale Adapter left on the same bucket by a restart,
// means the Adapter's view is wrong, so a move that lost to it fails rather
// than move a root back over a newer value.
var ownRoots = []string{
	RootReplayBase,
	RootCheckpoint,
	RootReplayCursor,
	RootReplayPayloads,
	RootPending,
}

// retryMovedLocked runs move, and when it failed with ErrRootMoved because
// the import root alone moved by appending, reads that root again and runs
// move once more. The caller holds rootMtx.
//
// Invariant: the retry runs only when every root in ownRoots still holds what
// held says, and the import root's new block reaches the block held references
// through its Prev links, so every root the Adapter read is still held.
// HoldImport is the one writer outside an Adapter, and it only appends that
// way. move releases an imported root only when it reads, at the move, that
// the Adapter holds it elsewhere, so the retry keeps every root the append
// added. Any other move, such as a stale Adapter's rewrite of a root alone,
// returns ErrRootMoved unchanged and the move changes nothing: that rewrite
// may have released a root the move would reference.
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

// readImportLocked reads the import root again when another writer moved it
// since the Adapter last read it, so a trim weighs every root an import holds.
// Only HoldImport and the Adapter's trims move the root, so the stored chain is
// the truth. Cost: one root read per trim, and one chain read after an import.
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

// appendsTo reports whether the import root block ref differs from held and
// reaches it through at most maxRootLinks Prev links, as the blocks HoldImport
// appends do. A chain that ends, or runs longer, before
// reaching held is not an append the retry can trust. The caller holds
// rootMtx.
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

// readRetained reads the retained roots the retained root holds. The caller
// holds rootMtx.
func (a *Adapter) readRetained(ctx context.Context) error {
	// Read the block the root holds, then decode its roots.
	data, err := a.readRootBlock(ctx, RootRetained, a.held[RootRetained])
	if err != nil || data == nil {
		return err
	}
	msg := &RetainedRootSet{}
	if err := msg.UnmarshalVT(data); err != nil {
		return err
	}
	a.retainedHeld = msg.GetRoots()
	return nil
}

// addPayloads holds extra beside the payloads already held. A local writer
// calls it before the operation that references them is queued, so the swap
// stores the writer's own kept payload pages in its one transaction, and the
// lock waits only for another swap. Apply calls it for each operation it
// applies, so the call holds nothing new for an operation this device already
// holds. requireHeld refuses a payload nothing holds now with
// block.ErrRootNotHeld: a local writer sets requireHeld, since a stage released
// before the admit may have left its tree to the collector.
func (a *Adapter) addPayloads(ctx context.Context, extra []*block.BlockRef, requireHeld bool) error {
	release, err := a.lockRoots(ctx)
	if err != nil {
		return err
	}
	defer release()
	return a.appendRoots(ctx, &a.payloads, extra, requireHeld)
}

// holdRetained holds the retained roots under the retained root, or releases
// it when there are none.
func (a *Adapter) holdRetained(ctx context.Context, roots []*RetainedRoot) error {
	// Skip a set the root holds already, before storing any page. Only the
	// driver goroutine changes the held set, so it stays the same after the
	// check.
	unlock, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	same := slices.EqualFunc(roots, a.retainedHeld, (*RetainedRoot).EqualVT)
	unlock()
	if same {
		return nil
	}

	// Store the retained Worlds' kept pages, then hold them.
	refs := make([]*block.BlockRef, len(roots))
	for i, root := range roots {
		refs[i] = root.GetRootRef()
	}
	release, err := a.lockRoots(ctx, refs...)
	if err != nil {
		return err
	}
	defer release()

	// Release the name when nothing is retained.
	if len(roots) == 0 {
		if err := a.moveRootsLocked(ctx, nil, block.NamedRoot{Name: RootRetained}); err != nil {
			return err
		}
		a.retainedHeld = nil
		return nil
	}

	// Hold them under one block.
	entry, err := refsEntry(&RetainedRootSet{Roots: roots}, refs)
	if err != nil {
		return err
	}
	if err := a.moveRootsLocked(ctx, []*block.PutBatchEntry{entry}, block.NamedRoot{Name: RootRetained, Ref: entry.Ref}); err != nil {
		return err
	}
	a.retainedHeld = slices.Clone(roots)
	return nil
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

// buildLookupOp resolves operation types as OpTypes.LookupOn does, on the
// bus unless lookup is disabled.
func (a *Adapter) buildLookupOp() world.LookupOp {
	b := a.opts.Bus
	if a.opts.DisableLookup {
		b = nil
	}
	return a.opts.OpTypes.LookupOn(b, a.le, a.opts.EngineID)
}

// bucketRef returns head bound to the SharedObject block store's bucket.
func (a *Adapter) bucketRef(head *bucket.ObjectRef) *bucket.ObjectRef {
	ref := head.CloneVT()
	if ref == nil {
		ref = &bucket.ObjectRef{}
	}
	ref.BucketId = a.bucketID
	return ref
}

// newCursor builds a cursor at head over store, the PendingStore or a store
// that reads it, with the head's transform and the shared decoded block
// cache.
//
// The cursor's transform is the Space's, so the block store's linked files
// encode with that transform. The store reads no file until the store has a
// transform.
func (a *Adapter) newCursor(ctx context.Context, store block.StoreOps, head *bucket.ObjectRef) (*bucket_lookup.Cursor, error) {
	// Build the cursor.
	cursor, err := newWorldCursor(ctx, a.le, a.opts.Bus, a.opts.StepFactorySet, store, a.bucketID, a.decoded, head)
	if err != nil {
		return nil, err
	}

	// Give the linked files and the stored page count the Space's transform.
	a.storedPages.set(cursor.GetTransformer())
	if files := a.so.GetBlockStore().GetFileSegments(); files != nil {
		files.SetTransformer(cursor.GetTransformer())
	}
	return cursor, nil
}

// newWorldCursor builds a cursor at the Space World head over store, whose
// bucket is bucketID, with the head's transform and the decoded block cache
// decoded, which may be nil.
func newWorldCursor(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	store block.StoreOps,
	bucketID string,
	decoded *block.DecodedBlockCache,
	head *bucket.ObjectRef,
) (*bucket_lookup.Cursor, error) {
	// Build the transformer of the head's transform.
	conf := head.GetTransformConf()
	if len(conf.GetSteps()) == 0 {
		return nil, sobject.ErrEmptyTransformConfig
	}
	xfrm, err := newWorldTransformer(controller.ConstructOpts{Logger: le}, sfs, conf)
	if err != nil {
		return nil, err
	}

	// Bind the cursor to the bucket, resolving implicit references through it.
	ref := head.CloneVT()
	ref.BucketId = bucketID
	cursor := bucket_lookup.NewCursor(ctx, b, le, sfs, store, xfrm, ref,
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: bucketID}, conf)
	cursor.SetBucketIDOverride(bucketID)
	if decoded != nil {
		cursor.SetDecodedBlockCache(decoded)
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

// _ is a type assertion
var (
	_ world.Authority = (*Adapter)(nil)
	_ sobject.Body    = (*Adapter)(nil)
)
