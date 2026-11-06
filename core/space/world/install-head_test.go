package space_world

import (
	"context"
	"encoding/binary"
	"errors"
	"maps"
	"slices"
	"strconv"
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
// moves, and every move of the root named failOn.
type flakyRootStore struct {
	block.StoreOps
	// roots are the held roots by name.
	roots map[string]*block.BlockRef
	// fail counts the root moves left to fail.
	fail int
	// failOn, when set, names a root whose every move fails.
	failOn string
}

// GetRoot returns the block name holds.
func (s *flakyRootStore) GetRoot(_ context.Context, name string) (*block.BlockRef, error) {
	return s.roots[name], nil
}

// SetRoots stores entries and moves the roots, unless a failure is due.
func (s *flakyRootStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if s.fail > 0 {
		s.fail--
		return errors.New("injected root move failure")
	}
	if slices.ContainsFunc(roots, func(r block.NamedRoot) bool { return r.Name == s.failOn }) {
		return errors.New("injected root move failure")
	}
	if _, err := s.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	for _, root := range roots {
		if root.Ref.GetEmpty() {
			delete(s.roots, root.Name)
			continue
		}
		s.roots[root.Name] = root.Ref
	}
	return nil
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
	if store.roots[RootRetained].GetEmpty() {
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
	if err := a.trimHistories(ctx); err != nil {
		t.Fatal(err)
	}

	// The checkpoint root still holds the later World, and only it.
	if store.roots[RootCheckpoint].GetEmpty() || !a.checkpointWorlds.holds(later) || a.checkpointWorlds.holds(earlier) {
		t.Fatalf("checkpoint history after the install = %v", a.checkpointWorlds.roots)
	}
	if !store.roots[RootReplayBase].EqualsRef(earlier) {
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
	if err := a.trimHistories(ctx); err != nil {
		t.Fatal(err)
	}

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
		opened:           openedNow(),
		le:               logrus.NewEntry(log),
		pages:            block.NewPendingStore(cursor.GetBucket(), maxPending),
		held:             make(map[string]*block.BlockRef),
		checkpointWorlds: worldHistory{name: RootCheckpoint},
		cursorWorlds:     worldHistory{name: RootReplayCursor},
		pendingWorlds:    worldHistory{name: RootPending},
	}
	return a, collect
}

// TestAdmitWaitsOnlyForTheRootSwap checks, at two sizes of a checkpoint's
// kept World, that a local write's payload hold landing while the driver
// stores that World under the checkpoint root waits only for the root swap,
// not for the store: its longest wait stays a small part of the store's time
// and does not grow with the World.
func TestAdmitWaitsOnlyForTheRootSwap(t *testing.T) {
	type result struct{ wait, store time.Duration }
	results := make(map[int]result)
	for _, mib := range []int{4, 32} {
		var r result
		for range 3 {
			wait, store := measureAdmitWait(t, mib)
			r.wait, r.store = max(r.wait, wait), max(r.store, store)
		}
		t.Logf("checkpoint of %d MiB: store %v, longest payload hold wait %v", mib, r.store, r.wait)
		results[mib] = r
	}
	large := results[32]
	if large.wait > large.store/4 {
		t.Fatalf("a payload hold waited %v behind a %v checkpoint store", large.wait, large.store)
	}
}

// measureAdmitWait stores a kept World of mib MiB under the checkpoint root,
// as Root does, and returns how long a payload hold, as Admit makes it, waited
// once it landed during the store, and how long the store took.
func measureAdmitWait(t *testing.T, mib int) (wait, store time.Duration) {
	// Keep a World of mib MiB of 4 KiB pages, two levels deep, and a payload.
	ctx := t.Context()
	a, _ := newVolumeAdapter(t, (mib+8)<<20)
	put := func(data []byte, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, Refs: children, RefsKnown: true}}); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	var nodes []*block.BlockRef
	page := make([]byte, 4<<10)
	for n := range mib << 8 / 256 {
		leaves := make([]*block.BlockRef, 256)
		for i := range leaves {
			binary.LittleEndian.PutUint64(page, uint64(n<<8|i)) //nolint:gosec // small and positive.
			leaves[i] = put(slices.Clone(page))
		}
		nodes = append(nodes, put([]byte("node "+strconv.Itoa(n)), leaves...))
	}
	world := put([]byte("checkpoint World"), nodes...)
	payload := put([]byte("payload"))
	a.replay = &spaceReplay{a: a, base: &InnerState{}}
	a.replay.positions = []spacePosition{{state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: world}}}}

	// Start the checkpoint's store, then hold the payload during it.
	stored := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := a.Root(ctx, 1)
		stored <- err
	}()
	time.Sleep(2 * time.Millisecond)
	adm := &spaceAdmission{payloads: []*block.BlockRef{payload}, done: make(chan struct{})}
	held := time.Now()
	if err := a.holdAdmitted(ctx, adm); err != nil {
		t.Fatal(err)
	}
	wait = time.Since(held)
	if err := <-stored; err != nil {
		t.Fatal(err)
	}
	return wait, time.Since(start)
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
	if err := a.trimHistories(ctx); err != nil {
		t.Fatal(err)
	}
	if !a.cursorWorlds.holds(saved) || store.roots[RootReplayCursor].GetEmpty() {
		t.Fatalf("cursor history after the install = %v, want the saved World", a.cursorWorlds.roots)
	}
}

// TestTrimMovesRootsTogether checks that a trim moves the replay base and
// every history in one root move: a failed move leaves every root and history
// as it was, and the next trim lands them all.
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

	// A trim whose move of the pending root fails moves no root.
	before := maps.Clone(store.roots)
	store.failOn = RootPending
	if err := a.trimHistories(ctx); err == nil {
		t.Fatal("the trim landed through a failed root move")
	}
	store.failOn = ""
	if !maps.EqualFunc(store.roots, before, refEqual) || !a.cursorWorlds.holds(covered) || len(a.pendingWorlds.roots) != 0 {
		t.Fatalf("a failed trim moved roots: %v, cursor %v, pending %v", store.roots, a.cursorWorlds.roots, a.pendingWorlds.roots)
	}

	// The next trim lands every move.
	if err := a.trimHistories(ctx); err != nil {
		t.Fatal(err)
	}
	if !refEqual(store.roots[RootReplayBase], base) || a.cursorWorlds.holds(covered) || !a.cursorWorlds.holds(head) || !a.pendingWorlds.holds(candidate) {
		t.Fatalf("roots after the trim = %v, cursor %v, pending %v", store.roots, a.cursorWorlds.roots, a.pendingWorlds.roots)
	}
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
