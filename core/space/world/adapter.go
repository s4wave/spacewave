package space_world

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

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
// Worlds never reach the Volume. An operation's payloads, which no World need
// reach, are held under the replay payloads root before the operation is
// queued, so the operation log never names a payload a restart cannot read.
//
// Invariants:
//   - The adopted checkpoint's World is held, by replay-base or checkpoint, at
//     every point, so a restart always finds the World replay starts from.
//   - The replay cursor record names a World only after replay-cursor holds
//     it.
//   - The writer head is the last pending candidate, or the installed head
//     when none is pending, and a candidate's base is the writer head when it
//     was admitted.
//   - The installed head never loses an operation an earlier installed head
//     applied, until replay places that operation again or refuses it.
//   - A candidate is decided only after every head watcher saw the head that
//     holds it.
//   - Every stored page a live World names is held, by a named root whose
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
	// bucketID is the SharedObject block store's bucket.
	bucketID string
	// decoded is the decoded block cache every World cursor shares.
	decoded *block.DecodedBlockCache
	// pages keeps the World pages in front of so's block store. The serving
	// engine and the replay engine read and write through it.
	pages *block.PendingStore
	// queue queues admitted operations, in admission order.
	queue *opQueue
	// backfill keeps a local copy of the World while this device chose it.
	backfill *spaceBackfill
	// replay holds the positions above the installed checkpoint. The driver
	// goroutine owns it.
	replay *spaceReplay
	// missing holds the operations an installed head applied that a reorder
	// put after the positions replay has applied again. The installed head
	// stays until it is empty. The driver goroutine owns it.
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
	// retainedHeld are the retained roots the retained root holds. Guarded
	// by rootMtx.
	retainedHeld []*RetainedRoot
	// cursorWorlds, pendingWorlds and checkpointWorlds are the Worlds the
	// replay cursor, pending and checkpoint roots hold, each until a
	// checkpoint covers them.
	cursorWorlds, pendingWorlds, checkpointWorlds rootSet
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
	// unmatched are this device's refused operations no admission matched,
	// by local ID, until a checkpoint covers them. Guarded by bcast.
	unmatched map[string]spacePosition
	// watchers receive each installed head. Guarded by bcast.
	watchers []*headWatch
	// installed is set once the driver installed the first checkpoint.
	// Guarded by bcast.
	installed bool
	// headHeight and headPositions place the installed head: the height of
	// its checkpoint and how many positions above it it follows. Guarded by
	// bcast.
	headHeight    uint64
	headPositions int
	// engine is the serving engine once built, until it retires. Guarded by
	// bcast.
	engine *Engine
	// storing holds the Worlds each background store keeps live, such as a
	// checkpoint's, a cursor save's or a trim's, under the store's own token
	// until it returns. The sweep and the trim keep them live. Guarded by
	// bcast.
	storing map[uint64][]*block.BlockRef
	// storingNext is the next background store's token. Guarded by bcast.
	storingNext uint64
	// stopped reports that Execute's run ended, so no background store
	// starts. Guarded by bcast.
	stopped bool
	// background counts the background stores. Execute waits for them before
	// it releases the Space, so none outlives the adapter's claim.
	background sync.WaitGroup
	// runCtx ends every background store when Execute's run ends.
	runCtx context.Context
	// cursorSaving reports that a cursor save runs in the background.
	cursorSaving atomic.Bool
	// trimMtx runs the background trims one at a time.
	trimMtx csync.Mutex
	// trimSeq numbers the trims the driver gathers; a trim older than the
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

	// Choose the store the kept pages land in: the SharedObject's block
	// store, or the one the options give.
	store := so.GetBlockStore()
	var pagesStore block.StoreOps = store
	if opts.pagesStore != nil {
		pagesStore = opts.pagesStore
	}

	// Front it with the kept pages and the shared decoded block cache.
	decoded := store.GetDecodedBlockCache()
	if decoded == nil {
		decoded = block.NewDecodedBlockCache()
	}
	a := &Adapter{
		le:        le,
		opened:    make(chan struct{}),
		so:        so,
		opts:      opts,
		bucketID:  store.GetID(),
		decoded:   decoded,
		pages:     block.NewPendingStore(pagesStore, opts.MaxPendingBytes),
		queue:     &opQueue{so: so},
		missing:   make(map[string]struct{}),
		unmatched: make(map[string]spacePosition),
		fences:    make(map[string]*spaceFence),
		held:      make(map[string]*block.BlockRef),

		cursorWorlds:     rootSet{name: RootReplayCursor},
		pendingWorlds:    rootSet{name: RootPending},
		checkpointWorlds: rootSet{name: RootCheckpoint},
		payloads:         rootSet{name: RootReplayPayloads},
	}
	a.lookupOp = a.buildLookupOp()
	a.replay = &spaceReplay{a: a}
	a.backfill = newSpaceBackfill(a)

	// A transient replay claims nothing and reads no roots, so its driver
	// steps it at once.
	if opts.transient {
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
	detach, err := a.so.SetBody(ctx, a)
	if err != nil {
		return err
	}
	defer detach()
	runCtx, stopRun := context.WithCancel(ctx)
	defer stopRun()
	a.runCtx = runCtx

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
	if err := a.replay.openStage(ctx); err != nil {
		return err
	}
	close(a.opened)

	// Run the queue and the storage reclaim.
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return a.queue.execute(ctx, a.queued) })
	g.Go(func() error { return a.executeStorageReclaim(ctx) })

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

	// Refuse new background stores, end the running ones and wait for them,
	// so none starts or lands beside the next adapter's claim.
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.stopped = true
	})
	stopRun()
	a.background.Wait()
	a.replay.close()
	a.replay.endStage()
	return err
}

// errAdapterStopped is returned by a background store asked of an adapter
// whose run ended.
var errAdapterStopped = errors.New("space world adapter stopped")

// startBackground starts a background store that keeps roots live, under its
// own token, so a sweep keeps their pages for the store, and Execute waits for
// the store until the returned done is called once. It reports false, and
// starts nothing, once Execute's run ended.
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
		a.background.Add(1)
		token, ok = a.storingNext, true
		a.storingNext++
		a.storing[token] = slices.Clone(roots)
	})
	if !ok {
		return func() {}, false
	}

	// Drop them and the count once.
	var once sync.Once
	return func() {
		once.Do(func() {
			a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				delete(a.storing, token)
			})
			a.background.Done()
		})
	}, true
}

// backgroundContext returns ctx, also ended when Execute's run ends, for a
// background store.
func (a *Adapter) backgroundContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
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
// one inside QueueOperation, so its durability is unknown; the next open
// replays its operation if the SharedObject holds it.
func (a *Adapter) endAdmissions() {
	// Supersede the ones the queue never passed on.
	waiting := a.queue.drain()
	for _, adm := range waiting {
		a.fail(adm, world.ErrSuperseded)
	}

	// Leave the rest unknown.
	var rest []*spaceAdmission
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rest = slices.Clone(a.pending)
	})
	a.decide(rest, world.Undecided, "", world.ErrDurabilityUnknown)
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
// for every hold first, so a holder never sees it close.
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

	// Wait for an initialized head at or past that order.
	var queued bool
	for {
		var installed, reached bool
		var state *InnerState
		var waitCh <-chan struct{}
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			installed, state, waitCh = a.installed, a.headState, getWaitCh()
			reached = a.headHeight > target.Checkpoint.GetHeight() ||
				(a.headHeight == target.Checkpoint.GetHeight() && a.headPositions >= len(target.Order))
		})
		if !state.GetHeadRef().GetEmpty() && reached {
			break
		}

		// Initialize a World that replay of the opening order left empty.
		if installed && reached && !queued {
			init, err := a.initOp(ctx)
			if err != nil {
				return nil, err
			}
			if init != nil {
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
	engine := &Engine{Engine: bengine, a: a, cursor: cursor}
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.engine, a.readers = engine, engine
		broadcast()
	})
	return engine, nil
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
// refuse every operation it writes. The turn's Base is the writer head, not
// durable while a candidate admitted before it waits.
func (a *Adapter) Begin(ctx context.Context) (world.Turn, error) {
	// Refuse a participant that cannot write.
	if err := a.checkWrites(ctx); err != nil {
		return nil, err
	}

	// Take the turn at the writer head.
	release, err := a.turnMtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	var base world.Head
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		base = a.writer
		base.Durable = base.Root.EqualsRef(a.head.Root) && base.Seqno == a.head.Seqno
	})
	return &spaceTurn{a: a, base: base, release: release}, nil
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
// is live, so a sweep keeps its pages and a trim holds it; store flushes its
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
		if err := r.recompute(ctx, n); err != nil {
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

		// Hold it beside every World returned since the last install,
		// unless an install already made it the replay base, which holds
		// it.
		release, err := a.lockRoots(ctx, root)
		if err != nil {
			return err
		}
		defer release()
		if refEqual(a.held[RootReplayBase], root) {
			return nil
		}
		return a.addRoot(ctx, &a.checkpointWorlds, root)
	}
	return data, store, nil
}

// Apply computes the World after op at index. An operation of this device
// whose candidate was built on the World before index takes the candidate's
// root without running op again; any other runs on the replay engine. When the
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
	if c := adm.getCandidate(); c != nil && op.Refused == "" && refEqual(c.Base, prev.GetHeadRef().GetRootRef()) {
		state := prev.CloneVT()
		state.HeadRef.RootRef = c.Root.Clone()
		state.HeadRef.BucketId = ""
		adopt = &spacePosition{state: state, seqno: c.Seqno, payloads: adm.payloads}
	}

	// Compute the position, waiting at an operation whose type a plugin the
	// World installs declares until the plugin registers a handler.
	pos, err := r.run(ctx, index, op, adopt)
	var pending *pendingOpError
	for !a.opts.transient && errors.As(err, &pending) {
		if err := a.awaitOpHandler(ctx, pending); err != nil {
			return "", err
		}
		pos, err = r.run(ctx, index, op, adopt)
	}
	if err != nil {
		return "", err
	}
	r.positions = append(r.positions, pos)
	delete(a.missing, string(op.Hash))

	// Decide or place the candidate, and install the head when it lost
	// nothing.
	if adm != nil {
		a.place(adm, pos, adopt == nil)
	} else {
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

// Install moves the replay base to checkpoint's World and trims the
// histories, keeping the positions after the covered ones when the World
// after them is the checkpoint's.
func (a *Adapter) Install(ctx context.Context, checkpoint *sobject.SOCheckpointInner, covered int) (bool, error) {
	// Wait for the claim.
	if err := a.waitOpened(ctx); err != nil {
		return false, err
	}

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
	}
	if kept {
		r.positions = slices.Clone(r.positions[covered:])
		r.floor, r.saved = max(0, r.floor-covered), max(0, r.saved-covered)
	} else {
		if err := a.missAbove(ctx); err != nil {
			return false, err
		}
		r.positions, r.floor, r.saved = nil, 0, 0
	}
	r.base, r.baseSeqno, r.height = base, seqno, checkpoint.GetHeight()
	if len(r.restored) != 0 && !kept && !r.restoredFrom.EqualVT(base) {
		r.restored = nil
	}

	// Hold the checkpoint's World as the replay base, and trim the Worlds it
	// covers from the histories, in one root move.
	if err := a.trimInBackground(ctx); err != nil {
		return false, err
	}

	// Record the install and drop the pages no live World reaches.
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.installed = true
		broadcast()
	})
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

	// Run it beside the apply loop, keeping the Worlds it holds and its base
	// live until it returns, so a sweep keeps their pages for its flush. A
	// run that ended trims nothing more; the next mount does.
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
// A live World kept in memory may name stored pages that only the old base or
// an earlier history World holds, so when the base moves or any history
// releases a World, the live Worlds are stored and held in their place: the
// Worlds in memory under the cursor root and the candidates under the pending
// root. The Worlds whose pages left memory stay where a history holds them.
// The base, the holds and every release are one root move, so the trim
// releases only Worlds that no held or live World reaches through its child
// lists, and a failed move releases nothing.
//
// Cost per install: one pass over the live Worlds, keyed by ref, and one
// root move that stores the kept pages of the live Worlds.
func (a *Adapter) runTrim(ctx context.Context, p *trimPlan) error {
	// Leave the roots alone when the base stays and no history releases a
	// World.
	keep := func(root *block.BlockRef) bool { return !p.releases(root) }
	release, err := a.lockRoots(ctx, slices.Concat(p.memory, p.candidates, []*block.BlockRef{p.base})...)
	if err != nil {
		return err
	}
	defer release()
	histories := []*rootSet{&a.cursorWorlds, &a.pendingWorlds, &a.checkpointWorlds}
	moved := !refEqual(a.held[RootReplayBase], p.base)
	if !moved && !slices.ContainsFunc(histories, func(h *rootSet) bool { return h.releases(keep) }) {
		return nil
	}

	// Hold the live Worlds, release the rest and move the base together.
	next := [][]*block.BlockRef{
		a.cursorWorlds.held(keep, p.memory),
		a.pendingWorlds.held(keep, p.candidates),
		a.checkpointWorlds.held(keep, nil),
	}
	return a.setRootSets(ctx, histories, next, nil, block.NamedRoot{Name: RootReplayBase, Ref: p.base})
}

// missAbove notes as missing every applied position whose operation is still
// above the checkpoint, so the installed head stays until replay applies
// them again.
func (a *Adapter) missAbove(ctx context.Context) error {
	// Read the operations above the checkpoint now.
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return err
	}

	// Note those the replay applied.
	for _, pos := range a.replay.positions {
		if pos.reason == "" && set.Get(pos.op.Hash) != nil {
			a.missing[string(pos.op.Hash)] = struct{}{}
		}
	}
	for key := range a.missing {
		if set.Get([]byte(key)) == nil {
			delete(a.missing, key)
		}
	}
	return nil
}

// newAdmission returns the admission of op, with candidate when a local
// writer built a World for it.
func (a *Adapter) newAdmission(candidate world.Candidate, op []byte, payloads []*block.BlockRef) *spaceAdmission {
	return &spaceAdmission{a: a, candidate: candidate, op: op, payloads: payloads, done: make(chan struct{})}
}

// enqueue admits the encoded operation op, which builds no World of its own,
// after this device's admitted operations and queues it. A nil op is an
// acknowledgment.
func (a *Adapter) enqueue(op []byte) *spaceAdmission {
	adm := a.newAdmission(world.Candidate{}, op, nil)
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
		a.pending = append(a.pending, adm)
		if !adm.candidate.Root.GetEmpty() {
			a.writer = world.Head{Root: adm.candidate.Root, Seqno: adm.candidate.Seqno}
		}
	})
}

// holdAdmitted holds what adm's queued operation needs once it is durable:
// its payloads under the replay payloads root, and its candidate's World under
// the pending root when the kept pages outgrew memory, so the pages its
// writer's stage held stay held after the turn ends. Each hold is one swap
// that stores the writer's own pages, and waits on the root lock only for
// another swap, never for the driver's store of its pages.
func (a *Adapter) holdAdmitted(ctx context.Context, adm *spaceAdmission) error {
	// Hold the payloads before the operation that names them is queued.
	if len(adm.payloads) != 0 {
		if err := a.addPayloads(ctx, adm.payloads); err != nil {
			return err
		}
	}

	// Hold an outgrown candidate beside the earlier ones.
	if !a.pages.Full() || adm.candidate.Root.GetEmpty() {
		return nil
	}
	release, err := a.lockRoots(ctx)
	if err != nil {
		return err
	}
	defer release()
	return a.addRoot(ctx, &a.pendingWorlds, adm.candidate.Root)
}

// match returns this device's pending admission of op, or nil: the one the
// driver placed op for before, or the first queued one with op's data.
func (a *Adapter) match(op *sobject.OrderedOperation) *spaceAdmission {
	if op.Inner.GetPeerId() != a.so.GetPeerID().String() {
		return nil
	}
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

// noteUnmatched records a refused operation of this device that matched no
// admission, which its admission finds by local ID once QueueOperation
// returns.
func (a *Adapter) noteUnmatched(op *sobject.OrderedOperation, pos spacePosition) {
	if pos.reason == "" || op.Inner.GetPeerId() != a.so.GetPeerID().String() {
		return
	}
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.unmatched[op.Inner.GetLocalId()] = pos
	})
}

// getCandidate returns the admission's candidate, or nil for none.
func (s *spaceAdmission) getCandidate() *world.Candidate {
	if s == nil || s.candidate.Root.GetEmpty() {
		return nil
	}
	return &s.candidate
}

// place records the position of adm's operation. An applied one is decided
// when the installed head holds it; a refused one is decided now: Superseded
// when replay ran it on a World other than its base, so the writer runs its
// change again on the head, and Rejected otherwise.
func (a *Adapter) place(adm *spaceAdmission, pos spacePosition, ranAgain bool) {
	// Keep an applied operation for the install.
	if pos.reason == "" {
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if adm.state != admissionDecided {
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

// queued ends an admission QueueOperation failed, an acknowledgment, which
// applies nothing for the driver to decide, and an operation the driver
// already refused before QueueOperation returned, which it could not find by
// data a refused operation does not carry.
func (a *Adapter) queued(adm *spaceAdmission, localID string, err error) {
	// End a failure or an acknowledgment.
	switch {
	case err != nil:
		a.fail(adm, err)
		return
	case adm.op == nil:
		a.decide([]*spaceAdmission{adm}, world.Accepted, "", nil)
		return
	}

	// Record the local ID, and decide an operation the driver already refused.
	var refused *spacePosition
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		adm.localID = localID
		if pos, ok := a.unmatched[localID]; ok {
			delete(a.unmatched, localID)
			refused = &pos
		}
	})
	if refused != nil {
		a.decide([]*spaceAdmission{adm}, world.Rejected, refused.reason, errors.Wrap(sobject.ErrRejectedOp, refused.reason))
	}
}

// Acknowledge queues an acknowledgment after every operation this device
// admitted, and returns once the SharedObject holds it.
func (a *Adapter) Acknowledge(ctx context.Context) error {
	// Queue it after the admitted operations, and wait for the SharedObject
	// to hold it.
	_, err := a.enqueue(nil).WaitDecided(ctx)
	return err
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
	for _, adm := range slices.Backward(a.pending) {
		if c := adm.getCandidate(); c != nil {
			return world.Head{Root: c.Root, Seqno: c.Seqno}
		}
	}
	return a.head
}

// installHead installs the World after the last position as the head, moves
// the writer head with it, holds its retained roots, reports it to every head
// watcher, then decides the candidates it holds.
func (a *Adapter) installHead(ctx context.Context) error {
	// Install the head and collect the placed candidates.
	state, seqno := a.replay.head()
	height, positions := a.replay.height, len(a.replay.positions)
	head := world.Head{Root: state.GetHeadRef().GetRootRef().Clone(), Seqno: seqno, Durable: true}
	var placed []*spaceAdmission
	var watchers []*headWatch
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Collect the placed candidates.
		for _, adm := range a.pending {
			if adm.state == admissionPlaced {
				placed = append(placed, adm)
			}
		}

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
// ends the flush. reach lists the roots the caller's swap may name; a page
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

// moveRootsLocked is moveRoots for a caller holding rootMtx.
func (a *Adapter) moveRootsLocked(ctx context.Context, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
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
	if err := a.pages.SetRoots(ctx, entries, moves); err != nil {
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

	// Read each root, and the payloads and Worlds their blocks name.
	for _, name := range heldRootNames {
		ref, err := a.pages.GetRoot(ctx, name)
		if err != nil {
			return err
		}
		if !ref.GetEmpty() {
			a.held[name] = ref
		}
	}
	for _, h := range []*rootSet{&a.cursorWorlds, &a.pendingWorlds, &a.checkpointWorlds, &a.payloads} {
		if err := a.readRootSet(ctx, h); err != nil {
			return err
		}
	}
	return a.readRetained(ctx)
}

// readRetained reads the retained roots the retained root holds. The caller
// holds rootMtx.
func (a *Adapter) readRetained(ctx context.Context) error {
	// Read the block the root holds, then decode its roots.
	data, err := a.readHeldBlock(ctx, RootRetained)
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

// addPayloads holds extra beside the payloads already held, before the
// operation that names them is queued. A local writer calls it, so the swap
// stores the writer's own kept payload pages in its one transaction, and the
// lock waits only for another swap.
func (a *Adapter) addPayloads(ctx context.Context, extra []*block.BlockRef) error {
	release, err := a.lockRoots(ctx)
	if err != nil {
		return err
	}
	defer release()
	return a.setRootSets(ctx, []*rootSet{&a.payloads}, [][]*block.BlockRef{slices.Concat(a.payloads.roots, extra)}, nil)
}

// resetPayloads holds the payloads of the positions above the checkpoint and
// of the pending admissions, dropping those a checkpoint covered. The driver
// goroutine calls it.
func (a *Adapter) resetPayloads(ctx context.Context) error {
	// Store the payloads gathered now, then take the root lock before the
	// gather, so an admission is either pending at the gather or adds its
	// payloads after the reset.
	release, err := a.lockRoots(ctx, a.gatherPayloads()...)
	if err != nil {
		return err
	}
	defer release()

	// Gather and hold them.
	return a.setRootSets(ctx, []*rootSet{&a.payloads}, [][]*block.BlockRef{a.gatherPayloads()}, nil)
}

// gatherPayloads returns the payloads of the positions above the checkpoint
// and of the pending admissions. The driver goroutine calls it.
func (a *Adapter) gatherPayloads() []*block.BlockRef {
	payloads := a.replay.payloads()
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, adm := range a.pending {
			payloads = append(payloads, adm.payloads...)
		}
	})
	return payloads
}

// holdRetained holds the retained roots under the retained root, or releases
// it when there are none.
func (a *Adapter) holdRetained(ctx context.Context, roots []*RetainedRoot) error {
	// Skip a set the root holds already.
	refs := make([]*block.BlockRef, len(roots))
	for i, root := range roots {
		refs[i] = root.GetRootRef()
	}
	release, err := a.lockRoots(ctx, refs...)
	if err != nil {
		return err
	}
	defer release()
	if slices.EqualFunc(roots, a.retainedHeld, (*RetainedRoot).EqualVT) {
		return nil
	}

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

// buildLookupOp composes the built-in operations with the bus lookup.
func (a *Adapter) buildLookupOp() world.LookupOp {
	var busLookup world.LookupOp
	if !a.opts.DisableLookup && a.opts.Bus != nil {
		busLookup = world.BuildLookupWorldOpFunc(a.opts.Bus, a.le, a.opts.EngineID)
	}
	return func(ctx context.Context, opTypeID string) (world.Operation, error) {
		var lookups []world.LookupOp
		if a.opts.OpTypes.Lookup != nil {
			lookups = append(lookups, a.opts.OpTypes.Lookup)
		}
		if busLookup != nil {
			lookups = append(lookups, busLookup)
		}
		return world.LookupOpSlice(lookups).LookupOp(ctx, opTypeID)
	}
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
func (a *Adapter) newCursor(ctx context.Context, store block.StoreOps, head *bucket.ObjectRef) (*bucket_lookup.Cursor, error) {
	// Build the transformer of the head's transform.
	conf := head.GetTransformConf()
	if len(conf.GetSteps()) == 0 {
		return nil, sobject.ErrEmptyTransformConfig
	}
	xfrm, err := a.newTransformer(conf)
	if err != nil {
		return nil, err
	}

	// Bind the cursor to the bucket, resolving implicit references through it.
	cursor := bucket_lookup.NewCursor(ctx, a.opts.Bus, a.le, a.opts.StepFactorySet, store, xfrm, a.bucketRef(head),
		&bucket.BucketOpArgs{BucketId: a.bucketID, VolumeId: a.bucketID}, conf)
	cursor.SetBucketIDOverride(a.bucketID)
	cursor.SetDecodedBlockCache(a.decoded)
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

// refEqual reports whether a and b name the same block, or both none.
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
