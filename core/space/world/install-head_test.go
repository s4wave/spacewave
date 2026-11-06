package space_world

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// flakyRootStore holds named roots in memory and fails the next fail root
// moves, and every move of the root named failOn. It is safe for a background
// trim and the test to use together.
type flakyRootStore struct {
	block.StoreOps
	// mtx guards the fields below.
	mtx sync.Mutex
	// roots are the held roots by name.
	roots map[string]*block.BlockRef
	// fail counts the root moves left to fail.
	fail int
	// failOn, when set, names a root whose every move fails.
	failOn string
	// failed, when set, receives a value at each failed move without
	// blocking.
	failed chan struct{}
	// moved records each root a move landed, as name and ref.
	moved []string
}

// GetRoot returns the block name holds.
func (s *flakyRootStore) GetRoot(_ context.Context, name string) (*block.BlockRef, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.roots[name], nil
}

// held returns a copy of the held roots.
func (s *flakyRootStore) held() map[string]*block.BlockRef {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return maps.Clone(s.roots)
}

// set sets failOn.
func (s *flakyRootStore) set(failOn string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.failOn = failOn
}

// SetRoots stores entries and moves the roots, unless a failure is due.
func (s *flakyRootStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Fail when a failure is due.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.fail > 0 || slices.ContainsFunc(roots, func(r block.NamedRoot) bool { return r.Name == s.failOn }) {
		if s.fail > 0 {
			s.fail--
		}
		if s.failed != nil {
			select {
			case s.failed <- struct{}{}:
			default:
			}
		}
		return errors.New("injected root move failure")
	}

	// Store the entries and move the roots.
	if _, err := s.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	for _, root := range roots {
		s.moved = append(s.moved, root.Name+"="+root.Ref.MarshalString())
		if root.Ref.GetEmpty() {
			delete(s.roots, root.Name)
			continue
		}
		s.roots[root.Name] = root.Ref
	}
	return nil
}

// trimNow runs a background trim of a and waits for it.
func trimNow(t *testing.T, a *Adapter) {
	t.Helper()
	if err := a.trimInBackground(t.Context()); err != nil {
		t.Fatal(err)
	}
	a.background.Wait()
}

// TestInstallHeadRetriesRetainedHold checks that a head whose retained-root
// hold failed is held and reported to the watchers by the next install, and
// that its placed candidate is decided only then.
func TestInstallHeadRetriesRetainedHold(t *testing.T) {
	// Build an adapter whose replay head retains a root, over a store that
	// fails the first root move.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef), fail: 1}
	retained, err := block.BuildBlockRef([]byte("retained World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err := block.BuildBlockRef([]byte("head World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &Adapter{
		opened: openedNow(),
		le:     logrus.NewEntry(logrus.New()),
		pages:  block.NewPendingStore(store, 1<<20),
		held:   make(map[string]*block.BlockRef),
	}
	a.replay = &spaceReplay{a: a, base: &InnerState{
		HeadRef:       &bucket.ObjectRef{RootRef: head},
		RetainedRoots: []*RetainedRoot{{Name: "export", RootRef: retained}},
	}}
	adm := &spaceAdmission{state: admissionPlaced, done: make(chan struct{})}
	a.pending = []*spaceAdmission{adm}
	var reports []world.Head
	a.watchers = []*headWatch{{fn: func(h world.Head) { reports = append(reports, h) }}}

	// The first install fails its hold, reports nothing and decides nothing.
	if err := a.installHead(ctx); err == nil {
		t.Fatal("the first install held the retained roots through a failing store")
	}
	if len(reports) != 0 || adm.state == admissionDecided {
		t.Fatalf("a failed install reported %d heads and decided %v", len(reports), adm.state == admissionDecided)
	}

	// The next install holds them, reports the head once, then decides.
	if err := a.installHead(ctx); err != nil {
		t.Fatal(err)
	}
	if store.held()[RootRetained].GetEmpty() {
		t.Fatal("the retry did not hold the retained roots")
	}
	if len(reports) != 1 || !reports[0].Root.EqualsRef(head) || adm.state != admissionDecided {
		t.Fatalf("the retry reported %v and decided %v", reports, adm.state == admissionDecided)
	}
}

// TestResetPayloadsSeesAdmissions checks that a payload reset waiting on the
// root lock holds the payloads of an admission that added them while it
// waited, so a queued operation never loses its payload hold.
func TestResetPayloadsSeesAdmissions(t *testing.T) {
	// Build an adapter with no replayed positions.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef)}
	a := &Adapter{
		opened: openedNow(),
		le:     logrus.NewEntry(logrus.New()),
		pages:  block.NewPendingStore(store, 1<<20),
		held:   make(map[string]*block.BlockRef),
	}
	a.replay = &spaceReplay{a: a, base: &InnerState{}}
	payload, err := block.BuildBlockRef([]byte("payload"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Start a reset while an admission holds the root lock.
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.resetPayloads(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// The admission is pending and holds its payloads, as Admit leaves it.
	adm := &spaceAdmission{state: admissionQueued, payloads: []*block.BlockRef{payload}, done: make(chan struct{})}
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.pending = append(a.pending, adm)
	})
	a.payloads = append(a.payloads, payload)
	release()

	// The reset holds them.
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(a.payloads) != 1 || !a.payloads[0].EqualsRef(payload) {
		t.Fatalf("payloads after the reset = %v, want the admission's", a.payloads)
	}
}

// TestInstallKeepsLaterCheckpointWorlds checks that an install of an earlier
// checkpoint keeps holding a later World Root returned while a kept position
// still is that World: Root stored its pages and dropped them from memory, so
// releasing it would leave the position naming unheld pages.
func TestInstallKeepsLaterCheckpointWorlds(t *testing.T) {
	// Build an adapter whose checkpoint root holds an earlier and a later
	// World, with a position at the later one.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef)}
	a := &Adapter{
		opened:           openedNow(),
		le:               logrus.NewEntry(logrus.New()),
		pages:            block.NewPendingStore(store, 1<<20),
		held:             make(map[string]*block.BlockRef),
		checkpointWorlds: worldHistory{name: RootCheckpoint},
		cursorWorlds:     worldHistory{name: RootReplayCursor},
		pendingWorlds:    worldHistory{name: RootPending},
	}
	earlier, err := block.BuildBlockRef([]byte("earlier World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	later, err := block.BuildBlockRef([]byte("later World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: earlier}}}
	a.replay.positions = []spacePosition{{state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: later}}}}
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addHistory(ctx, &a.checkpointWorlds, earlier); err != nil {
		t.Fatal(err)
	}
	if err := a.addHistory(ctx, &a.checkpointWorlds, later); err != nil {
		t.Fatal(err)
	}
	release()

	// Install the earlier one as the replay base.
	trimNow(t, a)

	// The checkpoint root still holds the later World, and only it.
	if store.held()[RootCheckpoint].GetEmpty() || !a.checkpointWorlds.holds(later) || a.checkpointWorlds.holds(earlier) {
		t.Fatalf("checkpoint history after the install = %v", a.checkpointWorlds.roots)
	}
	if !store.held()[RootReplayBase].EqualsRef(earlier) {
		t.Fatal("the install did not hold the replay base")
	}
}

// TestTrimHoldsPagesLiveWorldsName checks, on a Volume that frees at once,
// that an install whose trim releases a World, from the cursor or checkpoint
// history or by moving the replay base off it, first holds the live Worlds in
// memory in its place: the head, the writer head, and a World a reader of the
// engine still reads. The pages only the released World held, which the live
// Worlds still name, survive a collection. It fails without each hold,
// whatever the timing.
func TestTrimHoldsPagesLiveWorldsName(t *testing.T) {
	for _, name := range []string{RootReplayCursor, RootCheckpoint, RootReplayBase} {
		t.Run(name, func(t *testing.T) { runTrimHoldsPagesLiveWorldsName(t, name) })
	}
}

// runTrimHoldsPagesLiveWorldsName runs TestTrimHoldsPagesLiveWorldsName with
// the released World held by the root named name.
func runTrimHoldsPagesLiveWorldsName(t *testing.T, name string) {
	// Open an adapter on an in-memory Volume that frees at once.
	ctx := t.Context()
	a, collect := newVolumeAdapter(t, 1<<20)

	// Build three pages and a saved World over them.
	put := func(name string, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		data := []byte("trim " + name)
		for _, c := range children {
			data = append(data, c.MarshalString()...)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, Refs: children, RefsKnown: true}}); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	pages := []*block.BlockRef{put("head page"), put("writer page"), put("read page")}
	saved := put("saved World", pages...)

	// The root holds the saved World, which stores the pages and drops them
	// from memory.
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	switch name {
	case RootReplayCursor:
		err = a.addHistory(ctx, &a.cursorWorlds, saved)
	case RootCheckpoint:
		err = a.addHistory(ctx, &a.checkpointWorlds, saved)
	default:
		err = a.moveRootsLocked(ctx, nil, block.NamedRoot{Name: RootReplayBase, Ref: saved})
	}
	release()
	if err != nil {
		t.Fatal(err)
	}

	// The head names one page, the writer head, a candidate not yet
	// admitted, another, and a reader still reads a replaced head that names
	// the third.
	head, writer, read := put("head World", pages[0]), put("writer World", pages[1]), put("read World", pages[2])
	a.head, a.writer = world.Head{Root: head}, world.Head{Root: writer}
	a.readers = readRoots{read}
	base := put("base World")

	// A checkpoint installs at the base, with the head the only kept
	// position.
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: base}}}
	a.replay.positions = []spacePosition{{state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: head}}}}
	trimNow(t, a)

	// The base moved, the saved World is released, the live Worlds are held,
	// and the pages they name survive a collection.
	if !refEqual(a.held[RootReplayBase], base) {
		t.Fatalf("replay base after the trim = %v, want the base World", a.held[RootReplayBase])
	}
	for _, live := range []*block.BlockRef{head, writer, read} {
		if !a.cursorWorlds.holds(live) {
			t.Fatalf("cursor history after the trim = %v, missing %v", a.cursorWorlds.roots, live)
		}
	}
	if a.cursorWorlds.holds(saved) || a.checkpointWorlds.holds(saved) {
		t.Fatalf("histories after the trim still hold the saved World: cursor %v, checkpoint %v", a.cursorWorlds.roots, a.checkpointWorlds.roots)
	}
	collect()
	for _, page := range pages {
		if _, found, err := a.pages.GetBlock(ctx, page); err != nil || !found {
			t.Fatalf("collection freed a page a live World names: %v, %v", found, err)
		}
	}
}

// newVolumeAdapter returns an adapter whose PendingStore keeps at most
// maxPending bytes over a bucket of an in-memory Volume that frees at once,
// and a function that collects the Volume.
func newVolumeAdapter(t *testing.T, maxPending int) (*Adapter, func()) {
	// Open the bucket.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)

	// Build the adapter and the collection.
	vol := tb.Volume.(*volume_kvtx.Volume)
	collect := func() {
		t.Helper()
		for range 16 {
			res, err := vol.Collect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !res.More {
				return
			}
		}
	}
	a := &Adapter{
		le:               logrus.NewEntry(log),
		opened:           openedNow(),
		pages:            block.NewPendingStore(cursor.GetBucket(), maxPending),
		held:             make(map[string]*block.BlockRef),
		checkpointWorlds: worldHistory{name: RootCheckpoint},
		cursorWorlds:     worldHistory{name: RootReplayCursor},
		pendingWorlds:    worldHistory{name: RootPending},
	}
	return a, collect
}

// TestTrimKeepsRestoredWorlds checks that an install at mount, before replay
// applied any operation, keeps the World the saved cursor ends at under the
// cursor root: the restored positions name it, and no later save may hold it
// again before a collection.
func TestTrimKeepsRestoredWorlds(t *testing.T) {
	// Build an adapter whose cursor root holds the saved cursor's World, with
	// no position, head or reader yet.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef)}
	a := &Adapter{
		opened:           openedNow(),
		le:               logrus.NewEntry(logrus.New()),
		pages:            block.NewPendingStore(store, 1<<20),
		held:             make(map[string]*block.BlockRef),
		checkpointWorlds: worldHistory{name: RootCheckpoint},
		cursorWorlds:     worldHistory{name: RootReplayCursor},
		pendingWorlds:    worldHistory{name: RootPending},
	}
	base, err := block.BuildBlockRef([]byte("base World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := block.BuildBlockRef([]byte("saved World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addHistory(ctx, &a.cursorWorlds, saved); err != nil {
		t.Fatal(err)
	}
	release()
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: base}}}
	a.replay.restored = []spacePosition{{state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: saved}}}}

	// The install at mount keeps it.
	trimNow(t, a)
	if !a.cursorWorlds.holds(saved) || store.held()[RootReplayCursor].GetEmpty() {
		t.Fatalf("cursor history after the install = %v, want the saved World", a.cursorWorlds.roots)
	}
}

// TestTrimMovesRootsTogether checks that a background trim moves the replay
// base and every history in one root move: a failed move leaves every root
// and history as it was, and the trim tries again until it lands them all.
func TestTrimMovesRootsTogether(t *testing.T) {
	// Build an adapter whose cursor root holds a World the install covers,
	// with a pending candidate and a head, over a store that fails every move
	// of the pending root.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef)}
	a := &Adapter{
		opened:           openedNow(),
		le:               logrus.NewEntry(logrus.New()),
		pages:            block.NewPendingStore(store, 1<<20),
		held:             make(map[string]*block.BlockRef),
		checkpointWorlds: worldHistory{name: RootCheckpoint},
		cursorWorlds:     worldHistory{name: RootReplayCursor},
		pendingWorlds:    worldHistory{name: RootPending},
	}
	ref := func(name string) *block.BlockRef {
		t.Helper()
		r, err := block.BuildBlockRef([]byte(name), nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	covered, head, candidate, base := ref("covered World"), ref("head World"), ref("candidate World"), ref("base World")
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addHistory(ctx, &a.cursorWorlds, covered); err != nil {
		t.Fatal(err)
	}
	release()
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: base}}}
	a.replay.positions = []spacePosition{{state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: head}}}}
	a.head, a.writer = world.Head{Root: head}, world.Head{Root: candidate}
	a.pending = []*spaceAdmission{{candidate: world.Candidate{Root: candidate}}}

	// A background trim whose move of the pending root fails moves no root.
	before := store.held()
	store.failed = make(chan struct{}, 1)
	store.set(RootPending)
	if err := a.trimInBackground(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.failed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	release, err = a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	moved := !maps.EqualFunc(store.held(), before, refEqual) || !a.cursorWorlds.holds(covered) || len(a.pendingWorlds.roots) != 0
	release()
	if moved {
		t.Fatalf("a failed trim moved roots: %v", store.held())
	}

	// It tries again and lands every move once the store takes them.
	store.set("")
	a.background.Wait()
	if !refEqual(store.held()[RootReplayBase], base) || a.cursorWorlds.holds(covered) || !a.cursorWorlds.holds(head) || !a.pendingWorlds.holds(candidate) {
		t.Fatalf("roots after the trim = %v, cursor %v, pending %v", store.held(), a.cursorWorlds.roots, a.pendingWorlds.roots)
	}
}

// TestTrimKeepsItsBaseAndSkipsOlderPlans checks that a background trim keeps
// its base live until it runs, so a sweep after a later install keeps the
// base's pages for its flush, and that a trim gathered before a newer one is
// skipped, so the replay base never moves back onto an older checkpoint's
// World.
func TestTrimKeepsItsBaseAndSkipsOlderPlans(t *testing.T) {
	// Keep two checkpoint Worlds, each over a page of its own.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef)}
	a := &Adapter{
		opened:           openedNow(),
		le:               logrus.NewEntry(logrus.New()),
		pages:            block.NewPendingStore(store, 1<<20),
		held:             make(map[string]*block.BlockRef),
		checkpointWorlds: worldHistory{name: RootCheckpoint},
		cursorWorlds:     worldHistory{name: RootReplayCursor},
		pendingWorlds:    worldHistory{name: RootPending},
	}
	put := func(name string, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		data := []byte("plan " + name)
		for _, c := range children {
			data = append(data, c.MarshalString()...)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, Refs: children, RefsKnown: true}}); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	firstPage, secondPage := put("first page"), put("second page")
	first, second := put("first World", firstPage), put("second World", secondPage)

	// The first install gathers a trim onto the first World, which waits for
	// the trim turn.
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: first}}}
	turn, err := a.trimMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.trimInBackground(ctx); err != nil {
		t.Fatal(err)
	}

	// The second install moves the base on and sweeps; the first trim's base
	// keeps its page.
	a.replay.base = &InnerState{HeadRef: &bucket.ObjectRef{RootRef: second}}
	a.sweep()
	if _, found, err := a.pages.GetBlock(ctx, firstPage); err != nil || !found {
		t.Fatalf("a sweep dropped the page of a waiting trim's base: %v, %v", found, err)
	}

	// The second install gathers its trim; once the turn opens, the first is
	// skipped and the replay base moves onto the second World alone.
	if err := a.trimInBackground(ctx); err != nil {
		t.Fatal(err)
	}
	turn()
	a.background.Wait()
	if !refEqual(store.held()[RootReplayBase], second) {
		t.Fatalf("replay base after both trims = %v, want the second World", store.held()[RootReplayBase])
	}
	if slices.Contains(store.moved, RootReplayBase+"="+first.MarshalString()) {
		t.Fatalf("the older trim moved the replay base onto its World: %v", store.moved)
	}
}

// TestBackgroundStoresKeepTheirOwnRoots checks that each background store
// keeps its roots live under its own token, so one store's end leaves the
// same root live while another store holds it, and that a run that ended
// starts no store.
func TestBackgroundStoresKeepTheirOwnRoots(t *testing.T) {
	// Two stores hold the same root, as a trim's plan copies a store's.
	a := &Adapter{}
	a.replay = &spaceReplay{a: a, base: &InnerState{}}
	root, err := block.BuildBlockRef([]byte("stored World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	live := func() bool { return slices.ContainsFunc(a.gatherLive().memory, root.EqualsRef) }
	first, _ := a.startBackground(root)
	second, _ := a.startBackground(root)

	// The second's end leaves it live for the first; the first's end does
	// not.
	second()
	if !live() {
		t.Fatal("one store's end dropped a root another store holds")
	}
	first()
	if live() {
		t.Fatal("a root stayed live after every store holding it ended")
	}

	// A run that ended starts nothing.
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.stopped = true
	})
	if _, ok := a.startBackground(root); ok {
		t.Fatal("a store started after the run ended")
	}
	a.background.Wait()
}

// readRoots are the Worlds a test's engine readers still read.
type readRoots []*block.BlockRef

// ReadRoots returns the Worlds.
func (r readRoots) ReadRoots() []*block.BlockRef {
	return r
}

// openedNow returns the closed opened channel of an adapter a test builds
// directly, as Execute leaves it once it read the held roots, so Root, Apply
// and Install run at once.
func openedNow() chan struct{} {
	opened := make(chan struct{})
	close(opened)
	return opened
}
