package space_world

import (
	"bytes"
	"context"
	"slices"
	"sync"
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
//   - The writer head is the installed head or a candidate admitted after it,
//     and a candidate's base is the writer head when it was admitted.
//   - The installed head never loses an operation an earlier installed head
//     applied, until replay places that operation again or refuses it.
//   - A candidate is decided only after every head watcher saw the head that
//     holds it.
type Adapter struct {
	// le is the logger.
	le *logrus.Entry
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
	// rootMtx serializes the named root moves, which run I/O.
	rootMtx csync.Mutex
	// held are the named roots this adapter holds, by name. Guarded by
	// rootMtx.
	held map[string]*block.BlockRef
	// payloads are the payload roots the replay payloads root holds. Guarded
	// by rootMtx.
	payloads []*block.BlockRef
	// watchMtx serializes the head watchers' callbacks.
	watchMtx sync.Mutex

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

	// Front the SharedObject's block store with the kept pages.
	store := so.GetBlockStore()
	decoded := store.GetDecodedBlockCache()
	if decoded == nil {
		decoded = block.NewDecodedBlockCache()
	}
	a := &Adapter{
		le:        le,
		so:        so,
		opts:      opts,
		bucketID:  store.GetID(),
		decoded:   decoded,
		pages:     block.NewPendingStore(store, opts.MaxPendingBytes),
		queue:     &opQueue{so: so},
		missing:   make(map[string]struct{}),
		unmatched: make(map[string]spacePosition),
		fences:    make(map[string]*spaceFence),
		held:      make(map[string]*block.BlockRef),
	}
	a.lookupOp = a.buildLookupOp()
	a.replay = &spaceReplay{a: a}
	a.backfill = newSpaceBackfill(a)
	return a, nil
}

// engineIdleTimeout bounds the wait of a retired engine for the transactions
// open on it before it closes them.
const engineIdleTimeout = 30 * time.Second

// Execute serves the Space World until ctx ends: it reads the held roots and
// the saved cursor, runs the operation queue and the body driver, initializes
// an empty World as an owner, and builds the engine at the installed head.
func (a *Adapter) Execute(ctx context.Context) error {
	// Read what an earlier run held.
	if err := a.loadRoots(ctx); err != nil {
		return err
	}
	if err := a.loadCursor(ctx); err != nil {
		a.le.WithError(err).Warn("saved replay cursor is not readable, replaying from the checkpoint")
	}
	if err := a.backfill.start(ctx); err != nil {
		return err
	}

	// Run the queue, and attach the adapter to the SharedObject as its body,
	// which the SharedObject's driver replays the agreed order into.
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return a.queue.execute(ctx, a.queued) })
	detach := a.so.SetBody(a)
	defer detach()
	g.Go(func() error { return a.executeStorageReclaim(ctx) })

	// Serve the engine once the World is initialized.
	g.Go(func() error {
		// Build and publish the engine.
		engine, err := a.serve(ctx)
		if err != nil {
			return err
		}

		// Retire it when the run ends, let the transactions open on it
		// finish, then close it.
		<-ctx.Done()
		a.retire()
		idleCtx, cancel := context.WithTimeout(context.Background(), engineIdleTimeout)
		defer cancel()
		if err := engine.WaitIdle(idleCtx); err != nil {
			a.le.WithError(err).Warn("closing the World engine with transactions open")
		}
		return engine.close()
	})
	err := g.Wait()
	a.replay.close()
	return err
}

// WaitEngine waits for the engine serving the Space World.
func (a *Adapter) WaitEngine(ctx context.Context) (*Engine, error) {
	for {
		var engine *Engine
		var waitCh <-chan struct{}
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			engine, waitCh = a.engine, getWaitCh()
		})
		if engine != nil {
			return engine, nil
		}
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-waitCh:
		}
	}
}

// AcquireEngine waits for the serving engine and holds it open until release
// is called. The adapter retires the engine before it closes it, and waits
// for every hold first, so a holder never sees it close.
func (a *Adapter) AcquireEngine(ctx context.Context) (*Engine, func(), error) {
	for {
		// Hold the engine when one serves.
		var engine *Engine
		var waitCh <-chan struct{}
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			engine, waitCh = a.engine, getWaitCh()
			if engine != nil {
				a.holders++
			}
		})
		if engine != nil {
			return engine, a.releaseEngine, nil
		}

		// Wait for one.
		select {
		case <-ctx.Done():
			return nil, nil, context.Canceled
		case <-waitCh:
		}
	}
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
	for {
		var serving bool
		var waitCh <-chan struct{}
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			serving, waitCh = a.engine == engine, getWaitCh()
		})
		if !serving {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitCh:
		}
	}
}

// retire stops serving the engine and waits for every hold on it.
func (a *Adapter) retire() {
	for {
		var waitCh <-chan struct{}
		var held bool
		a.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if a.engine != nil {
				a.engine = nil
				broadcast()
			}
			held, waitCh = a.holders != 0, getWaitCh()
		})
		if !held {
			return
		}
		<-waitCh
	}
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
				adm := a.newAdmission(world.Candidate{}, init, nil)
				a.admit(adm)
				a.queue.push(adm)
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

	// Open the engine there and publish it.
	cursor, err := a.newCursor(ctx, state.GetHeadRef())
	if err != nil {
		return nil, err
	}
	bengine, err := world_block.NewEngine(ctx, a.le, cursor, a.lookupOp, a, a.opts.Verbose, world_block.WithRecordedOperations())
	if err != nil {
		cursor.Release()
		return nil, err
	}
	engine := &Engine{Engine: bengine, a: a, cursor: cursor}
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.engine = engine
		broadcast()
	})
	return engine, nil
}

// initOp returns the operation that initializes the World when this device
// owns the Space, or nil.
func (a *Adapter) initOp(ctx context.Context) ([]byte, error) {
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

	// Encode it.
	initOp, err := NewInitWorldOp(a.opts.InitWorldOp)
	if err != nil {
		return nil, err
	}
	return (&WorldOp{Body: &WorldOp_InitWorld{InitWorld: initOp}}).MarshalVT()
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

// Root moves the checkpoint root onto the World after the first n positions,
// storing its kept pages with the move, and returns its encoded InnerState.
func (a *Adapter) Root(ctx context.Context, n int) ([]byte, error) {
	// Answer only for an applied position, readable from memory.
	r := a.replay
	if n > len(r.positions) {
		return nil, nil
	}
	if n != 0 && n <= r.floor {
		// The World after n left memory unstored: compute it again.
		if err := r.recompute(ctx, n); err != nil {
			return nil, err
		}
	}
	state, _ := r.stateBefore(n)

	// Hold it, then return it.
	if err := a.moveRoots(ctx, nil, block.NamedRoot{Name: RootCheckpoint, Ref: state.GetHeadRef().GetRootRef()}); err != nil {
		return nil, err
	}
	return state.MarshalVT()
}

// Apply computes the World after op at index. An operation of this device
// whose candidate was built on the World before index takes the candidate's
// root without running op again; any other runs on the replay engine. When the
// positions hold every operation the installed head applied, the head after
// the last one is installed, and the candidates it holds are decided.
func (a *Adapter) Apply(ctx context.Context, index int, op *sobject.OrderedOperation) (string, error) {
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

	// Compute the position.
	pos, err := r.run(ctx, index, op, adopt)
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

// Install moves the replay base to checkpoint's World and releases the
// checkpoint root, keeping the positions after the covered ones when the
// World after them is the checkpoint's.
func (a *Adapter) Install(ctx context.Context, checkpoint *sobject.SOCheckpointInner, covered int) (bool, error) {
	// Decode the checkpoint's World.
	r := a.replay
	base := &InnerState{}
	if err := base.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		return false, errors.Wrap(err, "checkpoint World state")
	}

	// Hold it as the replay base, releasing the checkpoint root.
	baseRoot := base.GetHeadRef().GetRootRef()
	if err := a.moveRoots(ctx, nil,
		block.NamedRoot{Name: RootReplayBase, Ref: baseRoot},
		block.NamedRoot{Name: RootCheckpoint},
	); err != nil {
		return false, err
	}
	seqno, err := r.seqno(ctx, base)
	if err != nil {
		return false, err
	}

	// Keep the positions after the covered ones when the replay reached the
	// checkpoint's World there.
	kept := covered >= 0 && covered <= len(r.positions) && r.stateFor(covered).EqualVT(base)
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
	if len(r.restored) != 0 && !kept && !r.restoredBase().EqualVT(base) {
		r.restored = nil
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
// writer's stage held stay held after the turn ends.
func (a *Adapter) holdAdmitted(ctx context.Context, adm *spaceAdmission) error {
	// Hold the payloads before the operation that names them is queued.
	if len(adm.payloads) != 0 {
		if err := a.addPayloads(ctx, adm.payloads); err != nil {
			return err
		}
	}

	// Hold an outgrown candidate.
	if !a.pages.Full() || adm.candidate.Root.GetEmpty() {
		return nil
	}
	return a.moveRoots(ctx, nil, block.NamedRoot{Name: RootPending, Ref: adm.candidate.Root})
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
	// Queue it after the admitted operations.
	adm := a.newAdmission(world.Candidate{}, nil, nil)
	a.admit(adm)
	a.queue.push(adm)

	// Wait for the SharedObject to hold it.
	_, err := adm.WaitDecided(ctx)
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
		a.writer = a.head
		for _, adm := range slices.Backward(a.pending) {
			if c := adm.getCandidate(); c != nil {
				a.writer = world.Head{Root: c.Root, Seqno: c.Seqno}
				break
			}
		}
	})

	// Wake the waiters.
	for _, adm := range done {
		close(adm.done)
	}
}

// installHead installs the World after the last position as the head, holds
// its retained roots, reports it to every head watcher, then decides the
// candidates it holds.
func (a *Adapter) installHead(ctx context.Context) error {
	// Install the head and collect the placed candidates.
	state, seqno := a.replay.head()
	height, positions := a.replay.height, len(a.replay.positions)
	head := world.Head{Root: state.GetHeadRef().GetRootRef().Clone(), Seqno: seqno, Durable: true}
	var placed []*spaceAdmission
	var watchers []*headWatch
	var prevState *InnerState
	var changed bool
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
		changed = !state.EqualVT(a.headState) || a.head.Seqno != seqno
		if changed {
			prevState = a.headState
			a.head, a.headState = head, state
		}
		if changed || moved {
			broadcast()
		}
		watchers = slices.Clone(a.watchers)
	})

	// Hold changed retained roots.
	if changed && !slices.EqualFunc(state.GetRetainedRoots(), prevState.GetRetainedRoots(), (*RetainedRoot).EqualVT) {
		if err := a.holdRetained(ctx, state.GetRetainedRoots()); err != nil {
			return err
		}
	}

	// Report the head, then decide what it holds.
	if changed && !head.Root.GetEmpty() {
		a.watchMtx.Lock()
		for _, w := range watchers {
			w.fn(head)
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
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return a.moveRootsLocked(ctx, entries, roots...)
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

	// Read each root, and the payloads its block names.
	for _, name := range heldRootNames {
		ref, err := a.pages.GetRoot(ctx, name)
		if err != nil {
			return err
		}
		if !ref.GetEmpty() {
			a.held[name] = ref
		}
	}
	return a.readPayloads(ctx)
}

// readPayloads reads the payloads the replay payloads root holds. The caller
// holds rootMtx.
func (a *Adapter) readPayloads(ctx context.Context) error {
	// Read the block the root holds.
	ref := a.held[RootReplayPayloads]
	if ref.GetEmpty() {
		return nil
	}
	data, found, err := a.pages.GetBlock(ctx, ref)
	if err != nil || !found {
		return err
	}

	// Decode its payloads.
	msg := &HeldRoots{}
	if err := msg.UnmarshalVT(data); err != nil {
		return err
	}
	a.payloads = msg.GetRoots()
	return nil
}

// holdPayloads holds payloads under the replay payloads root, or releases
// the name when there are none. The caller holds rootMtx.
func (a *Adapter) holdPayloads(ctx context.Context, payloads []*block.BlockRef) error {
	// Release the name when nothing needs it.
	if len(payloads) == 0 {
		return a.moveRootsLocked(ctx, nil, block.NamedRoot{Name: RootReplayPayloads})
	}

	// Hold them under one block.
	entry, err := refsEntry(&HeldRoots{Roots: payloads}, payloads)
	if err != nil {
		return err
	}
	if err := a.moveRootsLocked(ctx, []*block.PutBatchEntry{entry}, block.NamedRoot{Name: RootReplayPayloads, Ref: entry.Ref}); err != nil {
		return err
	}
	a.payloads = payloads
	return nil
}

// addPayloads holds extra beside the payloads already held, before the
// operation that names them is queued.
func (a *Adapter) addPayloads(ctx context.Context, extra []*block.BlockRef) error {
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return a.holdPayloads(ctx, slices.Concat(a.payloads, extra))
}

// resetPayloads holds the payloads of the positions above the checkpoint and
// of the pending admissions, dropping those a checkpoint covered. The driver
// goroutine calls it.
func (a *Adapter) resetPayloads(ctx context.Context) error {
	// Gather them.
	payloads := a.replay.payloads()
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, adm := range a.pending {
			payloads = append(payloads, adm.payloads...)
		}
	})

	// Hold them.
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return a.holdPayloads(ctx, payloads)
}

// holdRetained holds the retained roots under the retained root, or releases
// it when there are none.
func (a *Adapter) holdRetained(ctx context.Context, roots []*RetainedRoot) error {
	// Release the name when nothing is retained.
	if len(roots) == 0 {
		return a.moveRoots(ctx, nil, block.NamedRoot{Name: RootRetained})
	}

	// Hold them under one block.
	refs := make([]*block.BlockRef, len(roots))
	for i, root := range roots {
		refs[i] = root.GetRootRef()
	}
	entry, err := refsEntry(&RetainedRootSet{Roots: roots}, refs)
	if err != nil {
		return err
	}
	return a.moveRoots(ctx, []*block.PutBatchEntry{entry}, block.NamedRoot{Name: RootRetained, Ref: entry.Ref})
}

// sweep drops the kept pages no live World reaches: the replay's Worlds and
// payloads, the installed head, the writer head and the pending candidates.
func (a *Adapter) sweep() {
	live := a.replay.liveRoots()
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		live = append(live, a.head.Root, a.writer.Root)
		for _, adm := range a.pending {
			live = append(live, adm.candidate.Root)
			live = append(live, adm.payloads...)
		}
		for _, root := range a.headState.GetRetainedRoots() {
			live = append(live, root.GetRootRef())
		}
	})
	a.pages.Sweep(live)
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

// newCursor builds a cursor at head over the PendingStore, with the head's
// transform and the shared decoded block cache.
func (a *Adapter) newCursor(ctx context.Context, head *bucket.ObjectRef) (*bucket_lookup.Cursor, error) {
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
	cursor := bucket_lookup.NewCursor(ctx, a.opts.Bus, a.le, a.opts.StepFactorySet, a.pages, xfrm, a.bucketRef(head),
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
