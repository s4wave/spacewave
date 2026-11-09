package packcopy_world

import (
	"context"
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

const (
	// WalkConcurrency is the reads a walk keeps in flight when its config
	// names none.
	WalkConcurrency = 64
	// WalkConcurrencyMax bounds the reads a walk keeps in flight.
	WalkConcurrencyMax = 1024
	// walkRetries is the rounds a walk tries its failed reads again.
	walkRetries = 3
	// walkRetryWait is the wait before the first retry round; each later
	// round waits longer.
	walkRetryWait = 5 * time.Second
	// walkFirstHoles is the holes a walk's status lists.
	walkFirstHoles = 20
	// walkAdmitEvery is the reads a walk worker does between waits on the
	// walk's gate.
	walkAdmitEvery = 64
	// walkReadTimeout bounds one read when the config names no bound.
	walkReadTimeout = 20 * time.Second
	// walkPersistEvery is how often a walk saves its position when the
	// config names no interval.
	walkPersistEvery = 30 * time.Second
	// walkPersistShare bounds the share of the walk's time spent saving its
	// position: after a save that took d, the next waits at least d times
	// this.
	walkPersistShare = 20
)

// Reasons a walk records a hole.
const (
	// HoleMissing is a block no source holds.
	HoleMissing = "missing"
	// HoleError is a block whose reads failed on every try.
	HoleError = "error"
	// HoleUntyped is a block read without its refs and of no known type.
	HoleUntyped = "untyped"
	// HoleUndecodable is a block read without its refs whose type did not
	// decode it.
	HoleUndecodable = "undecodable"
)

// Visited marks of a walk.
const (
	// walkSeen is a block queued or read.
	walkSeen uint8 = 1
	// walkUntyped is a block read without its refs and with no type, which a
	// later typed reach reads again.
	walkUntyped uint8 = 2
)

// Walk stages, the index of a stage's stack.
const (
	// walkStageWorld holds the World's trees and their okra pages.
	walkStageWorld = 0
	// walkStageObjects holds the Objects, their roots and their bodies.
	walkStageObjects = 1
)

// WalkSource is what a walk reads blocks from.
type WalkSource interface {
	// GetStoredBlock reads a block and the refs its store knows.
	GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error)
}

// DecodedStore stores a block read without its refs with the refs its type
// decodes, as provider_local.BlockStore does.
type DecodedStore interface {
	// StoreDecodedRefs stores data as ref's block with refs as its refs.
	StoreDecodedRefs(ctx context.Context, ref *block.BlockRef, data []byte, refs []*block.BlockRef) error
}

// WalkHole is a block a walk could not read or learn the refs of.
type WalkHole struct {
	// Ref is the block.
	Ref *block.BlockRef
	// Parent is the block that named it, nil for a pinned root.
	Parent *block.BlockRef
	// Reason is one of the Hole constants.
	Reason string
	// Detail is the error behind the hole, empty for none.
	Detail string
}

// WalkConfig is what a walk reads through and what it does with each block.
type WalkConfig struct {
	// Source is what the walk reads each block from.
	Source WalkSource
	// Store is what the walk lists the pinned head's object types through.
	Store block.StoreOps
	// RootTypes types the root of each listed object by its type.
	RootTypes RootTypes
	// Concurrency is the reads kept in flight; zero uses WalkConcurrency,
	// or the saved walk's when resuming.
	Concurrency int
	// Keep, when set, stores each block read without its refs with the refs
	// its type decodes, since the read path serves such a block without
	// keeping it.
	Keep DecodedStore
	// KeepAll passes Keep every block read, also those whose store knows
	// their refs, which the walk otherwise does not keep.
	KeepAll bool
	// KeepSlots, when above zero, bounds the keeps running at once. A keep
	// that finds every slot busy is counted deferred and not stored; the
	// next walk reads the block again.
	KeepSlots int32
	// Locate, when set, reports which blocks the World's own packs hold. The
	// walk does not keep a block they hold: the World's local store serves
	// every block without its refs, so a read alone cannot tell a block the
	// World holds from one only the read fallback holds.
	Locate func(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
	// Gate, when set, is waited on before reads, every walkAdmitEvery reads
	// of each worker, and returns an error that stops the walk.
	Gate func(ctx context.Context) error
	// Holes, when set, receives each hole. A resumed walk may report a hole
	// its earlier run reported after its last saved position.
	Holes func(h WalkHole)
	// ReadTimeout bounds one read; zero uses 20 seconds.
	ReadTimeout time.Duration
	// StatePath, when set, is the file the walk saves its position in, and
	// the file ResumeWalk reads.
	StatePath string
	// PersistEvery is how often the walk saves its position; zero uses 30
	// seconds.
	PersistEvery time.Duration
}

// walkItem is one block a walk will read.
type walkItem struct {
	// ref is the block.
	ref *block.BlockRef
	// parent is the block that named it, nil for a pinned root.
	parent *walkNode
	// typ is the block's type, nil when unknown.
	typ any
	// stage is the stage the block belongs to.
	stage uint8
	// okraValue marks an okra entry's value.
	okraValue bool
}

// walkNode is a block the walk read, kept while a block it named waits, so
// a saved position names each waiting block's parent. A node whose type a
// save cannot name keeps its own parent, so a resume can type it from its
// path; a node does not change once built.
type walkNode struct {
	// ref is the block.
	ref *block.BlockRef
	// parent is the block that named it, nil for a pinned root or a node
	// whose type a save names.
	parent *walkNode
	// typ is the block's type, nil when unknown.
	typ any
	// stage is the stage the block was read in.
	stage uint8
}

// walkKey is a block's key in a walk's visited set.
type walkKey [32]byte

// keyOf returns ref's key: its digest, or a digest of the marshaled hash for a
// hash of another size.
func keyOf(ref *block.BlockRef) walkKey {
	// A digest of the key's size is the key.
	var k walkKey
	h := ref.GetHash().GetHash()
	if len(h) == len(k) {
		copy(k[:], h)
		return k
	}

	// Any other hash is digested.
	data, _ := ref.GetHash().MarshalVT()
	return sha256.Sum256(data)
}

// walkSample is a read count at a time.
type walkSample struct {
	// at is when the count was taken.
	at time.Time
	// reads is the count.
	reads uint64
}

// Walk reads every block reachable from a pinned World head through a source,
// many reads at a time. A block's type comes from its parent; a block read
// without its refs is decoded by its type for them and, with a Keep store,
// stored with them. The World's trees and their okra pages are read before
// object roots and bodies; the object types are listed between the two, from
// the graph the first stage kept. A hole is recorded and the walk goes on.
//
// With a StatePath, the walk saves its position every PersistEvery, when its
// phase changes and when Run returns: the pin, the phase, the counts, and
// every block it will read with its type and the block that named it. Every
// block reachable from the pin that the walk has not read is reachable from
// the saved blocks, since a block stays in flight until it has queued each
// of its children, so ResumeWalk continues from the last save, reading again
// at most what the walk read since. A type of a kind the save cannot name is
// not saved: the save keeps the path of blocks to it from the nearest block
// whose type it names, and a resume reads that path again and decodes each
// block by its parent's type.
type Walk struct {
	// le logs the walk's progress.
	le *logrus.Entry
	// conf is the walk's config.
	conf WalkConfig
	// conc is the reads kept in flight.
	conc int
	// resume reads the frontier from the state file when Run starts.
	resume bool
	// reg maps the block types a save names to their constructors.
	reg map[string]block.Ctor
	// keeping counts the keeps running.
	keeping atomic.Int32
	// persisted, when set, is called after each save; tests wait on it.
	persisted func()
	// persistMtx serializes the saves.
	persistMtx sync.Mutex

	// mtx guards the fields below it.
	mtx sync.Mutex
	// cond wakes workers waiting for a block to read.
	cond *sync.Cond
	// state is the saved header: the pin, the phase, the start and the
	// resumes. Its status is unset; status holds the counts.
	state *WalkState
	// typer types the blocks: the World's trees alone until the objects are
	// listed, then the objects' roots too.
	typer *Typer
	// stacks hold the blocks to read by stage, read last in first out.
	stacks [2][]walkItem
	// retry holds the failed reads for the next retry round.
	retry []walkItem
	// held holds the Objects reached before the listing.
	held []walkItem
	// flying holds the reads running by id.
	flying map[uint64]walkItem
	// nextID numbers the reads.
	nextID uint64
	// visited marks each block reached by this run.
	visited map[walkKey]uint8
	// visitedBefore counts the blocks earlier runs reached, less those this
	// run queued again from the saved position.
	visitedBefore uint64
	// inFlight counts the reads running by stage.
	inFlight [2]int
	// final marks the last retry round, whose failures are holes.
	final bool
	// status is the walk's progress.
	status *WalkStatus
	// rate holds recent samples of the read count, for the read rate.
	rate []walkSample
	// persistTook is how long the last save took.
	persistTook time.Duration
}

// NewWalk builds a walk of the World pin names, from its roots.
func NewWalk(le *logrus.Entry, conf WalkConfig, pin *WalkPin) (*Walk, error) {
	if len(pin.GetHeads()) == 0 || pin.GetHeads()[0].GetRootRef().GetEmpty() {
		return nil, errors.New("the walk's pin names no World root")
	}
	state := &WalkState{
		Pin:         pin.CloneVT(),
		Phase:       WalkPhase_WALK_PHASE_WORLD,
		Concurrency: uint32(walkConc(conf.Concurrency, 0)), //nolint:gosec // bounded by WalkConcurrencyMax.
		StartedAt:   time.Now().UnixMilli(),
	}
	return newWalk(le, conf, state, &WalkStatus{}, false), nil
}

// ResumeWalk builds a walk that continues the position conf.StatePath holds.
// It reads the file's header; Run reads the rest. A walk the file records as
// done is refused.
func ResumeWalk(le *logrus.Entry, conf WalkConfig) (*Walk, error) {
	// Read the saved header, refusing a done walk.
	state, err := ReadWalkState(conf.StatePath)
	if err != nil {
		return nil, err
	}
	if state.GetPhase() == WalkPhase_WALK_PHASE_DONE {
		return nil, errors.New("the saved walk is done")
	}

	// Continue its counts as one more run, at the saved concurrency unless
	// the config names one.
	status := state.GetStatus().CloneVT()
	if status == nil {
		status = &WalkStatus{}
	}
	state.Status = nil
	state.Resumes++
	state.Concurrency = uint32(walkConc(conf.Concurrency, int(state.GetConcurrency()))) //nolint:gosec // bounded by WalkConcurrencyMax.
	return newWalk(le, conf, state, status, true), nil
}

// walkConc returns the reads a walk keeps in flight: conc, else saved, else
// WalkConcurrency, at most WalkConcurrencyMax.
func walkConc(conc, saved int) int {
	switch {
	case conc > 0:
	case saved > 0:
		conc = saved
	default:
		conc = WalkConcurrency
	}
	return min(conc, WalkConcurrencyMax)
}

// newWalk builds a walk from its saved header and counts.
func newWalk(le *logrus.Entry, conf WalkConfig, state *WalkState, status *WalkStatus, resume bool) *Walk {
	// Build the walk at the saved concurrency.
	w := &Walk{
		le:      le,
		conf:    conf,
		conc:    int(state.GetConcurrency()),
		resume:  resume,
		reg:     walkRegistry(conf.RootTypes),
		state:   state,
		flying:  make(map[uint64]walkItem),
		visited: make(map[walkKey]uint8),
		status:  status,
	}
	w.cond = sync.NewCond(&w.mtx)
	if w.conc <= 0 {
		w.conc = WalkConcurrency
	}

	// Show the pin in the status.
	w.status.Concurrency = uint32(w.conc) //nolint:gosec // bounded by WalkConcurrencyMax.
	w.status.Head = state.GetPin().GetHeads()[0].GetRootRef().MarshalString()
	w.status.Seqno = state.GetPin().GetSeqno()
	w.status.Resumes = state.GetResumes()
	if w.status.Stage == "" || w.status.Stage == "done" {
		w.status.Stage = "world"
	}

	// Fill in the bounds the config leaves unset.
	if conf.ReadTimeout <= 0 {
		w.conf.ReadTimeout = walkReadTimeout
	}
	if conf.PersistEvery <= 0 {
		w.conf.PersistEvery = walkPersistEvery
	}
	return w
}

// Pin returns the World the walk reads.
func (w *Walk) Pin() *WalkPin {
	return w.state.GetPin()
}

// StartedAt returns when the walk first started, in Unix milliseconds.
func (w *Walk) StartedAt() int64 {
	return w.state.GetStartedAt()
}

// Snapshot returns a copy of the walk's progress.
func (w *Walk) Snapshot() *WalkStatus {
	// Copy the counts and add the queue sizes.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	st := w.status.CloneVT()
	st.Visited = w.visitedBefore + uint64(len(w.visited))
	st.Queued = uint64(len(w.stacks[0]) + len(w.stacks[1]) + len(w.held))
	st.InFlight = uint32(w.inFlight[0] + w.inFlight[1]) //nolint:gosec // bounded by conc.
	st.RetryPending = uint64(len(w.retry))

	// Rate the reads over the samples of the last ten seconds.
	now := time.Now()
	w.rate = append(w.rate, walkSample{at: now, reads: st.Reads})
	for len(w.rate) > 2 && now.Sub(w.rate[1].at) > 10*time.Second {
		w.rate = w.rate[1:]
	}
	if first := w.rate[0]; now.Sub(first.at) > 0 && st.Reads > first.reads {
		st.BlocksPerSec = float64(st.Reads-first.reads) / now.Sub(first.at).Seconds()
	}
	return st
}

// Run walks until every block reachable from the pin is read, ctx ends or
// the gate fails. With a StatePath it saves the position as it goes and once
// more as it returns, whatever stopped it; a done walk saves its final counts
// with no frontier.
func (w *Walk) Run(ctx context.Context) error {
	// Type the World's trees, and the objects too when the listing ran.
	if err := w.retype(ctx, w.state.GetPhase() != WalkPhase_WALK_PHASE_WORLD); err != nil {
		return err
	}

	// Queue the saved frontier, or the pinned roots, and save the start.
	if w.resume {
		if err := w.load(ctx); err != nil {
			return errors.Wrap(err, "resume the saved walk")
		}
	} else {
		w.pushRoots()
	}
	if err := w.persist(); err != nil {
		return err
	}

	// Save the position as the walk goes, then once more where it stopped.
	stop := w.persistLoop(ctx)
	err := w.stages(ctx)
	stop()
	if perr := w.persist(); perr != nil && err == nil {
		err = perr
	}
	return err
}

// pushRoots queues the pinned roots so the first is read first.
func (w *Walk) pushRoots() {
	heads := w.state.GetPin().GetHeads()
	for i := len(heads) - 1; i >= 0; i-- {
		root := heads[i].GetRootRef()
		if root.GetEmpty() {
			continue
		}
		w.push(walkItem{ref: root, typ: w.typer.RootType(root), stage: walkStageWorld})
	}
}

// retype builds the typer of the pinned roots, listing the objects' types
// from the pinned head when listed is set.
func (w *Walk) retype(ctx context.Context, listed bool) error {
	// Build the typer, with the object types once listed.
	roots := RootTypes{}
	if listed {
		roots = w.conf.RootTypes
	}
	typer, err := New(ctx, w.le, w.conf.Store, w.heads(), roots)
	if err != nil {
		return errors.Wrap(err, "type the world")
	}

	// Type the next reads with it.
	w.mtx.Lock()
	w.typer = typer
	w.mtx.Unlock()
	return nil
}

// heads returns the pinned roots.
func (w *Walk) heads() []*bucket.ObjectRef {
	return w.state.GetPin().GetHeads()
}

// stages runs the stages the phase has left.
func (w *Walk) stages(ctx context.Context) error {
	// Settle the world stage, its failed reads included, before typing the
	// objects: the listing reads the graph through the store, so a subtree
	// left behind a failed read would be read there one page at a time.
	w.mtx.Lock()
	phase := w.state.GetPhase()
	w.mtx.Unlock()
	if phase == WalkPhase_WALK_PHASE_WORLD {
		if err := w.settle(ctx); err != nil {
			return err
		}
		if err := w.typeObjects(ctx); err != nil {
			return err
		}
		if err := w.persist(); err != nil {
			return err
		}
	}

	// Read the objects, then record the end.
	if err := w.settle(ctx); err != nil {
		return err
	}
	w.mtx.Lock()
	w.state.Phase = WalkPhase_WALK_PHASE_DONE
	w.status.Stage = "done"
	w.status.DoneAt = time.Now().UnixMilli()
	w.mtx.Unlock()
	return nil
}

// settle reads until the stacks drain, then tries the failed reads again, up
// to walkRetries rounds; the last round's failures are holes.
func (w *Walk) settle(ctx context.Context) error {
	w.mtx.Lock()
	w.final = false
	w.mtx.Unlock()
	for round := 0; ; round++ {
		// Read until the stacks drain.
		if err := w.drain(ctx); err != nil {
			return err
		}

		// Take the failed reads for another round, unless that was the last.
		w.mtx.Lock()
		if len(w.retry) == 0 || w.final {
			w.mtx.Unlock()
			return nil
		}
		w.status.Stage = "retry"
		w.final = round+1 >= walkRetries
		retry := w.retry
		w.retry = nil
		for _, it := range retry {
			w.stacks[it.stage] = append(w.stacks[it.stage], it)
		}
		w.status.Retried += uint64(len(retry))
		w.mtx.Unlock()

		// Wait before reading them again; they stay queued, so a save
		// meanwhile keeps them.
		t := time.NewTimer(walkRetryWait * time.Duration(round+1))
		select {
		case <-ctx.Done():
			t.Stop()
			return context.Cause(ctx)
		case <-t.C:
		}
	}
}

// typeObjects lists the World's object types and queues the Objects held
// back until then. The caller runs no workers.
func (w *Walk) typeObjects(ctx context.Context) error {
	// List the types from the pinned head.
	w.mtx.Lock()
	w.status.Stage = "types"
	w.mtx.Unlock()
	w.le.Info("World block walk: reading the object types")
	if err := w.retype(ctx, true); err != nil {
		return err
	}

	// Queue the held Objects for the objects stage.
	w.mtx.Lock()
	w.state.Phase = WalkPhase_WALK_PHASE_OBJECTS
	w.status.Stage = "objects"
	w.stacks[walkStageObjects] = append(w.stacks[walkStageObjects], w.held...)
	w.le.WithField("objects", len(w.held)).Info("World block walk: the objects are typed")
	w.held = nil
	w.mtx.Unlock()
	return nil
}

// drain runs the workers until the stacks are empty and no read runs.
func (w *Walk) drain(ctx context.Context) error {
	// Wake the workers when ctx ends.
	stop := context.AfterFunc(ctx, func() {
		w.mtx.Lock()
		w.cond.Broadcast()
		w.mtx.Unlock()
	})
	defer stop()

	// Run the workers.
	var wg sync.WaitGroup
	var gateErr atomic.Pointer[error]
	for range w.conc {
		wg.Go(func() {
			for n := 0; ; n++ {
				if w.conf.Gate != nil && n%walkAdmitEvery == 0 {
					if err := w.conf.Gate(ctx); err != nil {
						gateErr.CompareAndSwap(nil, &err)
						w.mtx.Lock()
						w.cond.Broadcast()
						w.mtx.Unlock()
						return
					}
				}
				id, it, ok := w.next(ctx, &gateErr)
				if !ok {
					return
				}
				w.done(id, it, !w.visit(ctx, it))
			}
		})
	}
	wg.Wait()

	// Report why the workers stopped.
	if errp := gateErr.Load(); errp != nil {
		return *errp
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return nil
}

// next pops the next block to read, the world stage first, waiting while
// reads run that may queue more. It reports false when the walk is drained,
// ctx ended or the gate failed.
func (w *Walk) next(ctx context.Context, gateErr *atomic.Pointer[error]) (uint64, walkItem, bool) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	for {
		if ctx.Err() != nil || gateErr.Load() != nil {
			return 0, walkItem{}, false
		}
		for stage := range w.stacks {
			s := w.stacks[stage]
			if n := len(s); n != 0 {
				it := s[n-1]
				s[n-1] = walkItem{}
				w.stacks[stage] = s[:n-1]
				w.inFlight[stage]++
				w.nextID++
				w.flying[w.nextID] = it
				return w.nextID, it, true
			}
		}
		if w.inFlight[0]+w.inFlight[1] == 0 {
			w.cond.Broadcast()
			return 0, walkItem{}, false
		}
		w.cond.Wait()
	}
}

// done ends a read, queueing its block again when requeue is set, and marks
// the end of the world stage the first time it drains.
func (w *Walk) done(id uint64, it walkItem, requeue bool) {
	// End the read, queueing its block again when asked.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	delete(w.flying, id)
	w.inFlight[it.stage]--
	if requeue {
		w.stacks[it.stage] = append(w.stacks[it.stage], it)
	}

	// Mark the end of the world stage, and wake the workers.
	if w.status.GetWorldDoneAt() == 0 && len(w.stacks[walkStageWorld]) == 0 && w.inFlight[walkStageWorld] == 0 && w.state.GetPhase() == WalkPhase_WALK_PHASE_WORLD {
		w.status.WorldDoneAt = time.Now().UnixMilli()
		if w.status.GetStage() == "world" {
			w.status.Stage = "objects"
		}
		w.le.WithFields(WalkFields(w.status)).Info("World block walk: the world stage is done")
	}
	w.cond.Broadcast()
}

// push queues it unless the walk reached it before; a block read untyped
// without its refs is queued again when a typed parent names it. An Object
// reached before the listing waits for it.
func (w *Walk) push(it walkItem) {
	// Skip a block reached before, unless a typed parent now names a block
	// read untyped.
	k := keyOf(it.ref)
	w.mtx.Lock()
	defer w.mtx.Unlock()
	switch w.visited[k] {
	case walkSeen:
		return
	case walkUntyped:
		if it.typ == nil {
			return
		}
		w.status.Untyped--
	}

	// Queue it, or hold an Object for the listing.
	w.visited[k] = walkSeen
	if w.state.GetPhase() == WalkPhase_WALK_PHASE_WORLD && it.typ != nil && w.typer.Kind(it.typ) == KindObject {
		it.stage = walkStageObjects
		w.held = append(w.held, it)
		return
	}
	w.stacks[it.stage] = append(w.stacks[it.stage], it)
	w.cond.Signal()
}

// visit reads one block, counts it, and queues its children. It reports
// false when ctx ended before the block's children were queued, so the
// block goes back on its stack and a save keeps it.
func (w *Walk) visit(ctx context.Context, it walkItem) bool {
	// Read the block, bounding a read that waits on a slow source.
	rctx, cancel := context.WithTimeout(ctx, w.conf.ReadTimeout)
	stored, err := w.conf.Source.GetStoredBlock(rctx, it.ref)
	cancel()
	if ctx.Err() != nil {
		return false
	}
	switch {
	case errors.Is(err, block.ErrNotFound) || (err == nil && stored == nil):
		w.hole(it, HoleMissing, "")
		return true
	case err != nil:
		w.mtx.Lock()
		final := w.final
		if !final {
			w.retry = append(w.retry, it)
		}
		w.mtx.Unlock()
		if final {
			w.hole(it, HoleError, err.Error())
		}
		return true
	}

	// Learn the block's children: by its type when it has one, else from
	// the refs its store knows.
	w.mtx.Lock()
	typer := w.typer
	w.mtx.Unlock()
	kind := typer.Kind(it.typ)
	var kids []packcopy.TypedRef
	if it.typ != nil {
		typed, ok, derr := typer.Decode(ctx, it.typ, it.ref, stored.Data)
		switch {
		case derr == nil && ok:
			kids = typed
			if !stored.RefsKnown && !w.keepDecoded(ctx, it, stored.Data, typed) {
				return false
			}
		case !stored.RefsKnown:
			w.count(it, kind, stored)
			reason := ""
			if derr != nil {
				reason = derr.Error()
			}
			w.hole(it, HoleUndecodable, reason)
			return true
		}
	} else if !stored.RefsKnown {
		w.count(it, kind, stored)
		w.mtx.Lock()
		w.visited[keyOf(it.ref)] = walkUntyped
		w.mtx.Unlock()
		w.hole(it, HoleUntyped, "")
		return true
	}
	if stored.RefsKnown {
		kids = mergeStoredRefs(kids, stored.Refs)
		if w.conf.KeepAll && !w.keepDecoded(ctx, it, stored.Data, kids) {
			return false
		}
	}
	w.count(it, kind, stored)

	// Queue the children; an object's root starts the objects stage.
	node := &walkNode{ref: it.ref, typ: it.typ, stage: it.stage}
	if !savableType(w.reg, it.typ) {
		node.parent = it.parent
	}
	for _, kid := range kids {
		if kid.Ref.GetEmpty() {
			continue
		}
		stage := it.stage
		kidKind := typer.Kind(kid.Type)
		if kidKind == KindObjectRoot {
			stage = walkStageObjects
		}
		w.push(walkItem{
			ref:       kid.Ref,
			parent:    node,
			typ:       kid.Type,
			stage:     stage,
			okraValue: kind == KindOkraPage && kidKind != KindOkraPage,
		})
	}
	return true
}

// count counts a block read by its kind.
func (w *Walk) count(it walkItem, kind string, stored *block.StoredBlock) {
	// Count the read and its bytes.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	st := w.status
	st.Reads++
	st.Bytes += uint64(len(stored.Data))

	// Count it by its kind.
	switch kind {
	case KindOkraRoot:
		st.OkraRoots++
	case KindOkraPage:
		st.OkraPages++
	case KindObject:
		st.Objects++
	}
	if it.okraValue && kind != KindOkraPage {
		st.OkraValues++
	}
	if !stored.RefsKnown {
		st.RefsUnknown++
	}
}

// mergeStoredRefs adds the refs a store knows that the typed refs lack.
func mergeStoredRefs(typed []packcopy.TypedRef, refs []*block.BlockRef) []packcopy.TypedRef {
	// With no typed refs, the stored refs are the children, untyped.
	if len(typed) == 0 {
		out := make([]packcopy.TypedRef, 0, len(refs))
		for _, r := range refs {
			out = append(out, packcopy.TypedRef{Ref: r})
		}
		return out
	}

	// Add each stored ref the typed refs lack.
	seen := make(map[walkKey]struct{}, len(typed))
	for _, t := range typed {
		if !t.Ref.GetEmpty() {
			seen[keyOf(t.Ref)] = struct{}{}
		}
	}
	for _, r := range refs {
		if r.GetEmpty() {
			continue
		}
		if _, ok := seen[keyOf(r)]; !ok {
			typed = append(typed, packcopy.TypedRef{Ref: r})
		}
	}
	return typed
}

// keepDecoded stores a block read without its refs with the refs its type
// decoded, so the local store keeps it. It skips a block the World's packs
// hold, and defers one when every keep slot is busy, so the keeps take only
// the writer time the World leaves. It reports false when ctx ended before
// the keep finished, so the block is read again.
func (w *Walk) keepDecoded(ctx context.Context, it walkItem, data []byte, typed []packcopy.TypedRef) bool {
	// Keep nothing without a store.
	if w.conf.Keep == nil {
		return true
	}

	// Skip a block the World's packs hold. A failed lookup keeps the block.
	if w.conf.Locate != nil {
		held, err := w.conf.Locate(ctx, []*block.BlockRef{it.ref})
		if err == nil && len(held) == 1 && held[0] {
			w.mtx.Lock()
			w.status.KeepHeld++
			w.mtx.Unlock()
			return true
		}
	}

	// Take a keep slot, or leave the block for the next walk.
	if w.conf.KeepSlots > 0 {
		if w.keeping.Add(1) > w.conf.KeepSlots {
			w.keeping.Add(-1)
			w.mtx.Lock()
			w.status.KeepDeferred++
			w.mtx.Unlock()
			return true
		}
		defer w.keeping.Add(-1)
	}

	// Store the block with its decoded refs.
	refs := make([]*block.BlockRef, 0, len(typed))
	for _, t := range typed {
		if !t.Ref.GetEmpty() {
			refs = append(refs, t.Ref)
		}
	}
	err := w.conf.Keep.StoreDecodedRefs(ctx, it.ref, data, refs)
	if err != nil && ctx.Err() != nil {
		return false
	}

	// Count the keep, logging a failure.
	w.mtx.Lock()
	if err != nil {
		w.status.KeepFailed++
	} else {
		w.status.KeptDecoded++
	}
	w.mtx.Unlock()
	if err != nil {
		w.le.WithError(err).WithField("ref", it.ref.MarshalString()).Debug("World block walk: store decoded refs")
	}
	return true
}

// hole records a block the walk could not read or learn the refs of.
func (w *Walk) hole(it walkItem, reason, detail string) {
	// Name the parent.
	var parent *block.BlockRef
	if it.parent != nil {
		parent = it.parent.ref
	}

	// Count it by its reason.
	w.mtx.Lock()
	st := w.status
	switch reason {
	case HoleMissing:
		st.Missing++
	case HoleError:
		st.Errors++
	case HoleUntyped:
		st.Untyped++
	case HoleUndecodable:
		st.Undecodable++
	}

	// List it in the status.
	text := reason + " " + it.ref.MarshalString()
	if parent != nil {
		text += " parent " + parent.MarshalString()
	}
	if detail != "" {
		text += ": " + detail
	}
	if len(st.FirstHoles) < walkFirstHoles {
		st.FirstHoles = append(st.FirstHoles, text)
	}
	w.mtx.Unlock()

	// Report it.
	if w.conf.Holes != nil {
		w.conf.Holes(WalkHole{Ref: it.ref, Parent: parent, Reason: reason, Detail: detail})
	}
}

// WalkFields are the log fields of a walk's progress.
func WalkFields(st *WalkStatus) logrus.Fields {
	return logrus.Fields{
		"stage":        st.GetStage(),
		"reads":        st.GetReads(),
		"bytes":        st.GetBytes(),
		"okra-roots":   st.GetOkraRoots(),
		"okra-pages":   st.GetOkraPages(),
		"okra-values":  st.GetOkraValues(),
		"objects":      st.GetObjects(),
		"refs-unknown": st.GetRefsUnknown(),
		// known-refs is the blocks read with their refs, or kept with the
		// refs their type decoded.
		"known-refs":    st.GetReads() - st.GetRefsUnknown() + st.GetKeptDecoded(),
		"kept-decoded":  st.GetKeptDecoded(),
		"keep-failed":   st.GetKeepFailed(),
		"keep-held":     st.GetKeepHeld(),
		"keep-deferred": st.GetKeepDeferred(),
		"missing":       st.GetMissing(),
		"errors":        st.GetErrors(),
		"retried":       st.GetRetried(),
		"untyped":       st.GetUntyped(),
		"undecodable":   st.GetUndecodable(),
		"resumes":       st.GetResumes(),
		"frontier":      st.GetFrontier(),
	}
}
