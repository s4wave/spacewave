package space_world

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
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
	// exists counts the GetBlockExists calls.
	exists int
}

// GetBlockExists counts the call and asks the inner store.
func (s *flakyRootStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	s.mtx.Lock()
	s.exists++
	s.mtx.Unlock()
	return s.StoreOps.GetBlockExists(ctx, ref)
}

// existsCalls returns the GetBlockExists calls so far.
func (s *flakyRootStore) existsCalls() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.exists
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
	a.waitBackground()
}

// storedWorld stores a World block named name in store and returns its ref:
// a root set names only Worlds some store holds.
func storedWorld(t *testing.T, store block.StoreOps, name string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(name), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutBlockBatch(t.Context(), []*block.PutBatchEntry{{Ref: ref, Data: []byte(name), RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	return ref
}

// TestInstallHeadRetriesRetainedHold checks that a head whose retained-root
// hold failed is held and reported to the watchers by the next install, that
// its placed candidate is decided only then, and that a later install of the
// same retained set reads and moves nothing.
func TestInstallHeadRetriesRetainedHold(t *testing.T) {
	// Build an adapter whose replay head retains a root, over a store that
	// fails the first root move.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef), fail: 1}
	retained := storedWorld(t, store.StoreOps, "retained World")
	head := storedWorld(t, store.StoreOps, "head World")
	a := openedAdapter(logrus.NewEntry(logrus.New()), store, 1<<20)
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

	// An install whose retained set the root holds already stores nothing
	// and asks the store nothing.
	exists, moved := store.existsCalls(), len(store.moved)
	if err := a.installHead(ctx); err != nil {
		t.Fatal(err)
	}
	if n := store.existsCalls() - exists; n != 0 || len(store.moved) != moved {
		t.Fatalf("an install of a held retained set asked the store %d times and moved %d roots", n, len(store.moved)-moved)
	}
}

// TestResetPayloadsSeesAdmissions walks the replay payloads root through a
// Volume that frees every unheld block at once. A reset waiting on the root
// lock holds the payloads an admission added while it waited. A reset after
// the admission is decided releases them, and the next admission holds only
// its own. A payload page another payload shares survives the release of the
// set that named both. A reset that changes nothing keeps the stored root.
// An adapter opened again over the same roots keeps the stored payloads when
// it adds more.
func TestResetPayloadsSeesAdmissions(t *testing.T) {
	// Build an adapter with no replayed positions over the Volume.
	ctx := t.Context()
	a, collect := newVolumeAdapter(t, 1<<20, time.Nanosecond)
	a.replay = &spaceReplay{a: a, base: &InnerState{}}

	// put keeps a payload page naming children in a's pages, as a writer
	// keeps its pages.
	put := func(a *Adapter, name string, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		data := []byte("payload " + name)
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

	// stored returns the payloads the Volume's payloads root holds, read
	// through its chain of blocks as an open reads them.
	stored := func(a *Adapter) []*block.BlockRef {
		t.Helper()
		ref, err := a.pages.GetRoot(ctx, RootReplayPayloads)
		if err != nil {
			t.Fatal(err)
		}
		if !refEqual(ref, a.held[RootReplayPayloads]) {
			t.Fatalf("payloads root = %v, the adapter holds %v", ref, a.held[RootReplayPayloads])
		}
		read := rootSet{name: RootReplayPayloads}
		if err := a.readRootSet(ctx, &read); err != nil {
			t.Fatal(err)
		}
		return read.roots
	}

	// expect checks that a holds want under the payloads root, in memory and
	// in the Volume, then collects and checks that every block of live
	// survives.
	expect := func(a *Adapter, step string, want []*block.BlockRef, live ...*block.BlockRef) {
		t.Helper()
		if !slices.EqualFunc(a.payloads.roots, want, refEqual) || !slices.EqualFunc(stored(a), want, refEqual) {
			t.Fatalf("%s: payloads %v, stored %v, want %v", step, a.payloads.roots, stored(a), want)
		}
		collect()
		for _, ref := range live {
			if _, found, err := a.pages.GetBlock(ctx, ref); err != nil || !found {
				t.Fatalf("%s: collection freed held payload page %v: %v, %v", step, ref, found, err)
			}
		}
	}

	// admit records an admission of payloads as pending, as Admit does
	// before it holds them.
	admit := func(a *Adapter, payloads ...*block.BlockRef) *spaceAdmission {
		adm := &spaceAdmission{state: admissionQueued, payloads: payloads, done: make(chan struct{})}
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			a.pending = append(a.pending, adm)
		})
		return adm
	}

	// decide drops adm from the pending admissions.
	decide := func(adm *spaceAdmission) {
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			a.pending = slices.DeleteFunc(a.pending, func(o *spaceAdmission) bool { return o == adm })
		})
	}

	// Start a reset while the root lock is held, then admit a payload and
	// hold it under that lock, as addPayloads does.
	shared := put(a, "shared")
	first := put(a, "first", shared)
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.resetPayloads(ctx) }()
	time.Sleep(50 * time.Millisecond)
	adm := admit(a, first)
	if err := a.setRootSets(ctx, []*rootSet{&a.payloads}, [][]*block.BlockRef{{first}}, nil); err != nil {
		t.Fatal(err)
	}
	release()

	// The reset holds the admission's payload and the page it names.
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	expect(a, "reset beside an admission", []*block.BlockRef{first}, first, shared)

	// Once it is decided, a reset releases the name, and the collection
	// frees the payload, so the checks below see a collector that frees.
	decide(adm)
	if err := a.resetPayloads(ctx); err != nil {
		t.Fatal(err)
	}
	expect(a, "reset after the decision", nil)
	if _, found, err := a.pages.GetBlock(ctx, first); err != nil || found {
		t.Fatalf("collection kept the released payload: %v, %v", found, err)
	}

	// The next admission holds only its own payloads, which share a page.
	shared = put(a, "shared")
	second := put(a, "second", shared)
	third := put(a, "third", shared)
	adm = admit(a, second, third)
	if err := a.addPayloads(ctx, adm.payloads); err != nil {
		t.Fatal(err)
	}
	expect(a, "admission after the release", []*block.BlockRef{second, third}, second, third, shared)

	// Another admission adds a payload, then is decided: the reset moves the
	// root back to the set it left, and keeps the shared page.
	fourth := put(a, "fourth", shared)
	other := admit(a, fourth)
	if err := a.addPayloads(ctx, other.payloads); err != nil {
		t.Fatal(err)
	}
	if want := []*block.BlockRef{second, third, fourth}; !slices.EqualFunc(stored(a), want, refEqual) {
		t.Fatalf("payloads stored beside an admission = %v, want %v", stored(a), want)
	}
	decide(other)
	if err := a.resetPayloads(ctx); err != nil {
		t.Fatal(err)
	}
	expect(a, "reset back to the earlier set", []*block.BlockRef{second, third}, second, third, shared)

	// A reset that gathers the same payloads moves nothing.
	held := a.held[RootReplayPayloads]
	if err := a.resetPayloads(ctx); err != nil {
		t.Fatal(err)
	}
	if !refEqual(a.held[RootReplayPayloads], held) {
		t.Fatalf("an unchanged reset moved the payloads root from %v to %v", held, a.held[RootReplayPayloads])
	}
	expect(a, "unchanged reset", []*block.BlockRef{second, third}, second, third, shared)

	// An adapter opened again over the same roots reads the payloads the
	// root holds, and adds a payload beside them.
	b := openedAdapter(a.le, a.pages.StoreOps, 1<<20)
	if err := b.loadRoots(ctx); err != nil {
		t.Fatal(err)
	}
	fifth := put(b, "fifth", shared)
	if err := b.addPayloads(ctx, admit(b, fifth).payloads); err != nil {
		t.Fatal(err)
	}
	expect(b, "admission after the open", []*block.BlockRef{second, third, fifth}, second, third, fifth, shared)

	// Each later admission writes one block naming only its own payload and
	// the block before, however many the root holds, and a reopen reads them
	// all back in order.
	want := []*block.BlockRef{second, third, fifth}
	for i := range 8 {
		extra := put(b, "payload "+strconv.Itoa(i))
		prev := b.held[RootReplayPayloads]
		if err := b.addPayloads(ctx, []*block.BlockRef{extra}); err != nil {
			t.Fatal(err)
		}
		want = append(want, extra)
		data, found, err := b.pages.GetBlock(ctx, b.held[RootReplayPayloads])
		if err != nil || !found {
			t.Fatalf("payload block: %v, %v", found, err)
		}
		held := &HeldRoots{}
		if err := held.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		if len(held.GetRoots()) != 1 || !held.GetRoots()[0].EqualsRef(extra) || !held.GetPrev().EqualsRef(prev) {
			t.Fatalf("admission %d wrote %v after %v, want only its payload after the last block", i, held, prev)
		}
	}
	// Adding payloads the set holds already, twice over and beside an empty
	// ref, moves nothing; adding them beside a new payload names only the new
	// one. The collection then keeps every payload the set holds.
	held = b.held[RootReplayPayloads]
	if err := b.addPayloads(ctx, []*block.BlockRef{second, want[4], second, nil}); err != nil {
		t.Fatal(err)
	}
	if !refEqual(b.held[RootReplayPayloads], held) {
		t.Fatalf("re-adding held payloads moved the payloads root from %v to %v", held, b.held[RootReplayPayloads])
	}
	sixth := put(b, "sixth", shared)
	if err := b.addPayloads(ctx, []*block.BlockRef{third, sixth, sixth, fifth}); err != nil {
		t.Fatal(err)
	}
	want = append(want, sixth)
	data, found, err := b.pages.GetBlock(ctx, b.held[RootReplayPayloads])
	if err != nil || !found {
		t.Fatalf("payload block: %v, %v", found, err)
	}
	mixed := &HeldRoots{}
	if err := mixed.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if len(mixed.GetRoots()) != 1 || !mixed.GetRoots()[0].EqualsRef(sixth) || !mixed.GetPrev().EqualsRef(held) {
		t.Fatalf("a mixed add wrote %v after %v, want only the new payload", mixed, held)
	}
	expect(b, "re-adding held payloads", want, append(slices.Clone(want), shared)...)

	reopened := rootSet{name: RootReplayPayloads}
	if err := b.readRootSet(ctx, &reopened); err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(reopened.roots, want, refEqual) || !slices.EqualFunc(b.payloads.roots, want, refEqual) {
		t.Fatalf("payloads read back = %v, held %v, want %v", reopened.roots, b.payloads.roots, want)
	}

	// An open that does not find the block the payloads root or a history
	// names fails: opening on an empty set would let the next move name the
	// unread block as Previous and release every root it names.
	world := put(b, "world", shared)
	release, err = b.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = b.addRoot(ctx, &b.cursorWorlds, world)
	release()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{RootReplayPayloads, RootReplayCursor} {
		c := openedAdapter(a.le, &hiddenBlockStore{StoreOps: b.pages.StoreOps, hide: b.held[name]}, 1<<20)
		err := c.loadRoots(ctx)
		if errors.Is(err, block.ErrNotFound) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}

		// Show what the open on an empty set costs: a payload and a World
		// added beside it release the unread block's roots.
		extra := put(c, "extra "+name)
		if err := c.addPayloads(ctx, admit(c, extra).payloads); err != nil {
			t.Fatal(err)
		}
		release, err := c.rootMtx.Lock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = c.addRoot(ctx, &c.cursorWorlds, extra)
		release()
		if err != nil {
			t.Fatal(err)
		}
		collect()
		for _, ref := range []*block.BlockRef{second, third, fifth, world, shared} {
			if _, found, err := b.pages.GetBlock(ctx, ref); err != nil || !found {
				t.Fatalf("open without the %s block: collection freed %v: %v, %v", name, ref, found, err)
			}
		}
		t.Fatalf("opened without the %s block", name)
	}
}

// hiddenBlockStore reports the block hide as not found, as a store does that
// lost or has not fetched the block a root names.
type hiddenBlockStore struct {
	block.StoreOps
	// hide is the block reported as not found.
	hide *block.BlockRef
}

// GetBlock reports hide as not found and reads every other block.
func (s *hiddenBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if ref.EqualsRef(s.hide) {
		return nil, false, nil
	}
	return s.StoreOps.GetBlock(ctx, ref)
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
	a := openedAdapter(logrus.NewEntry(logrus.New()), store, 1<<20)
	earlier := storedWorld(t, store.StoreOps, "earlier World")
	later := storedWorld(t, store.StoreOps, "later World")
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: earlier}}}
	a.replay.positions = []spacePosition{{state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: later}}}}
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addRoot(ctx, &a.checkpointWorlds, earlier); err != nil {
		t.Fatal(err)
	}
	if err := a.addRoot(ctx, &a.checkpointWorlds, later); err != nil {
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
	a, collect := newVolumeAdapter(t, 1<<20, time.Nanosecond)

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
		err = a.addRoot(ctx, &a.cursorWorlds, saved)
	case RootCheckpoint:
		err = a.addRoot(ctx, &a.checkpointWorlds, saved)
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
// maxPending bytes over a bucket of an in-memory Volume that frees once a
// record is grace old, and a function that collects the Volume. The
// SharedObject's block store is the same bucket.
func newVolumeAdapter(t *testing.T, maxPending int, grace time.Duration) (*Adapter, func()) {
	// Open the bucket.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: grace.String()},
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
	bkt := cursor.GetBucket()
	roots, ok := bkt.(block.LeasedRoots)
	proofs, pok := bkt.(block.RootProofs)
	if !ok || !pok {
		t.Fatal("the bucket holds no roots or proofs")
	}
	a := openedAdapter(logrus.NewEntry(log), bkt, maxPending)
	a.so = &bucketObject{store: &bucketStore{StoreOps: bkt, LeasedRoots: roots, RootProofs: proofs}}
	return a, collect
}

// bucketObject is a SharedObject whose block store is a bucket. Only
// GetBlockStore is implemented.
type bucketObject struct {
	sobject.SharedObject
	store bstore.BlockStore
}

// GetBlockStore returns the bucket.
func (o *bucketObject) GetBlockStore() bstore.BlockStore { return o.store }

// bucketStore is a SharedObject block store over a bucket, with its roots and
// proofs, and no storage backend.
type bucketStore struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs
}

// GetID returns the store's ID.
func (s *bucketStore) GetID() string { return "bucket" }

// GetDecodedBlockCache returns no cache.
func (s *bucketStore) GetDecodedBlockCache() *block.DecodedBlockCache { return nil }

// ReclaimStorage reports no storage backend.
func (s *bucketStore) ReclaimStorage(context.Context, bstore.ReclaimFence) (time.Time, error) {
	return time.Time{}, nil
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
	a := openedAdapter(logrus.NewEntry(logrus.New()), store, 1<<20)
	base := storedWorld(t, store.StoreOps, "base World")
	saved := storedWorld(t, store.StoreOps, "saved World")
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addRoot(ctx, &a.cursorWorlds, saved); err != nil {
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
	a := openedAdapter(logrus.NewEntry(logrus.New()), store, 1<<20)
	stored := func(name string) *block.BlockRef { return storedWorld(t, store.StoreOps, name) }
	covered, head, candidate, base := stored("covered World"), stored("head World"), stored("candidate World"), stored("base World")
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addRoot(ctx, &a.cursorWorlds, covered); err != nil {
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
	a.waitBackground()
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
	a := openedAdapter(logrus.NewEntry(logrus.New()), store, 1<<20)
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
	a.waitBackground()
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
	a.waitBackground()
}

// readRoots are the Worlds a test's engine readers still read.
type readRoots []*block.BlockRef

// ReadRoots returns the Worlds.
func (r readRoots) ReadRoots() []*block.BlockRef {
	return r
}

// openedAdapter returns an adapter over pages of inner with the clock and
// root sets NewAdapter gives it, already opened as Execute leaves it once it
// read the held roots, so Root, Apply and Install run at once.
func openedAdapter(le *logrus.Entry, inner block.StoreOps, maxPending int) *Adapter {
	opened := make(chan struct{})
	close(opened)
	return &Adapter{
		le:               le,
		opened:           opened,
		pages:            block.NewPendingStore(inner, maxPending),
		held:             make(map[string]*block.BlockRef),
		now:              time.Now,
		cursorWorlds:     rootSet{name: RootReplayCursor},
		pendingWorlds:    rootSet{name: RootPending},
		checkpointWorlds: rootSet{name: RootCheckpoint},
		payloads:         rootSet{name: RootReplayPayloads},
	}
}
