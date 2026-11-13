package world_block

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// errWindowRead ends WatchChanges after one report.
var errWindowRead = errors.New("read one change window")

// TestWatchChanges checks that the change feed compares the head a watcher saw
// against the current head and lists exactly the touched objects and quads.
// Objects revised only by their changed quads are reported through the quads.
func TestWatchChanges(t *testing.T) {
	// Create the objects the changes touch and one they leave alone.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	createWatchTestObject(t, ctx, ws, "watch/kept")
	createWatchTestObject(t, ctx, ws, "watch/deleted")
	createWatchTestObject(t, ctx, ws, "watch/linked")
	createWatchTestObject(t, ctx, ws, "watch/revised")
	createWatchTestObject(t, ctx, ws, "watch/untouched")

	// Link two of them with the quad the changes delete.
	unlinked := world.NewGraphQuadWithKeys("watch/kept", "<old>", "watch/linked", "")
	if err := ws.SetGraphQuad(ctx, unlinked); err != nil {
		t.Fatal(err)
	}

	// Watch from the current head.
	after := headSeqno(t, ctx, e)

	// Create and delete an object, and set and delete a quad, over several
	// commits.
	createWatchTestObject(t, ctx, ws, "watch/created")
	if _, err := ws.DeleteObject(ctx, "watch/deleted"); err != nil {
		t.Fatal(err)
	}
	linked := world.NewGraphQuadWithKeys("watch/kept", "<new>", "watch/created", "")
	if err := ws.SetGraphQuad(ctx, linked); err != nil {
		t.Fatal(err)
	}
	if err := ws.DeleteGraphQuad(ctx, unlinked); err != nil {
		t.Fatal(err)
	}

	// Revise one object explicitly, and one both explicitly and by a quad.
	for _, key := range []string{"watch/revised", "watch/kept"} {
		obj, err := world.MustGetObject(ctx, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		_, err = obj.IncrementRev(ctx)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Expect one window covering every commit.
	_, changes := readChangeWindow(t, ctx, e, after)
	if changes.Unknown {
		t.Fatal("changes after retained head are unknown")
	}
	got := slices.Sorted(slices.Values(changes.Keys))
	want := []string{"watch/created", "watch/deleted", "watch/kept", "watch/revised"}
	if !slices.Equal(got, want) {
		t.Fatalf("changed keys %q, want %q", got, want)
	}
	checkQuads(t, changes.Quads, linked, unlinked)
}

// TestWatchChangesImportedRoot checks that a head installed from outside the
// Engine, as a SharedObject replay or a snapshot import installs it, yields a
// precise ChangeSet when it shares the head's format, and an unknown one when
// it does not, since every graph key differs between formats.
func TestWatchChangesImportedRoot(t *testing.T) {
	for _, format := range []WorldFormat{WorldFormat_WORLD_FORMAT_CAYLEY_KV, WorldFormat_WORLD_FORMAT_NODE_GRAPH} {
		t.Run(format.String(), func(t *testing.T) {
			// Watch from the head the Engine opens at.
			ctx := t.Context()
			e := newFormatTestEngine(t, ctx, nil, format)
			after := headSeqno(t, ctx, e)

			// Import a World with more operations than the head and install it.
			stage, err := e.StageWorldState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer stage.Release()
			keys := []string{"import/a", "import/b", "import/c"}
			quads := []world.GraphQuad{
				world.NewGraphQuadWithKeys("import/a", "<p>", "import/b", ""),
				world.NewGraphQuadWithKeys("import/b", "<p>", "import/c", ""),
			}
			ref, err := ImportSnapshot(ctx, stage, func(yield func(string, block.Block) bool) {
				for _, key := range keys {
					if !yield(key, block_mock.NewExample(key)) {
						return
					}
				}
			}, quads)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.SetRootRef(ctx, ref); err != nil {
				t.Fatal(err)
			}

			// An import in another format than the head is unknown.
			_, changes := readChangeWindow(t, ctx, e, after)
			if format != importFormat {
				if !changes.Unknown {
					t.Fatalf("changes from a %v head to a %v import are known", format, importFormat)
				}
				return
			}

			// Otherwise expect the objects of both Worlds and the imported quads.
			if changes.Unknown {
				t.Fatal("changes to an imported root are unknown")
			}
			if got := slices.Sorted(slices.Values(changes.Keys)); !slices.Equal(got, keys) {
				t.Fatalf("changed keys %q, want %q", got, keys)
			}
			checkQuads(t, changes.Quads, quads...)
		})
	}
}

// TestWatchChangesLive checks that the change watch hands each commit to
// every subscriber already watching, listing only the object it changed.
func TestWatchChangesLive(t *testing.T) {
	// Start two subscribers from the current head.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	after := headSeqno(t, ctx, e)
	reports := make(chan []string)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			errs <- e.WatchChanges(ctx, after, nil, func(_ uint64, cs *world.ChangeSet) error {
				if cs.Unknown {
					return errors.New("live changes are unknown")
				}
				select {
				case reports <- slices.Sorted(slices.Values(cs.Keys)):
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}()
	}

	// Expect each commit at both subscribers. The second commit follows the
	// first report, so the change watch delivers it live.
	for _, key := range []string{"live/a", "live/b"} {
		createWatchTestObject(t, ctx, ws, key)
		for range 2 {
			select {
			case got := <-reports:
				if !slices.Equal(got, []string{key}) {
					t.Fatalf("changed keys %q, want %q", got, []string{key})
				}
			case err := <-errs:
				t.Fatal(err)
			case <-time.After(10 * time.Second):
				t.Fatalf("no report of %q", key)
			}
		}
	}
}

// TestWatchChangesRecorded checks that a subscriber starting behind a running
// change watch receives exactly the changes the watch listed since its head.
func TestWatchChangesRecorded(t *testing.T) {
	// Keep a live subscriber so the change watch runs and lists each commit.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	after := headSeqno(t, ctx, e)
	reported := make(chan uint64)
	go func() {
		_ = e.WatchChanges(ctx, after, nil, func(seqno uint64, _ *world.ChangeSet) error {
			select {
			case reported <- seqno:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	// Commit three objects, each compared by the watch before the next.
	keys := []string{"recorded/a", "recorded/b", "recorded/c"}
	for _, key := range keys {
		createWatchTestObject(t, ctx, ws, key)
		want := headSeqno(t, ctx, e)
		for seqno := uint64(0); seqno < want; {
			select {
			case seqno = <-reported:
			case <-time.After(10 * time.Second):
				t.Fatalf("watch did not report seqno %d", want)
			}
		}
	}

	// Expect a late subscriber from the first head to see all three.
	_, changes := readChangeWindow(t, ctx, e, after)
	if changes.Unknown {
		t.Fatal("recorded changes are unknown")
	}
	if got := slices.Sorted(slices.Values(changes.Keys)); !slices.Equal(got, keys) {
		t.Fatalf("changed keys %q, want %q", got, keys)
	}
}

// TestWatchChangesShareCatchUp checks that the subscribers that start behind
// the change watch at the same head share one comparison, and that a later one
// continues from that comparison through the spans the watch recorded since.
func TestWatchChangesShareCatchUp(t *testing.T) {
	// Commit a first head to watch from, since an empty root takes no pin.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	e := newVolumeFixture(t).engine
	ws := world.NewEngineWorldState(e, true)
	createWatchTestObject(t, ctx, ws, "share/first")
	after := headSeqno(t, ctx, e)
	afterRoot := e.GetRootRef().GetRootRef()

	// Commit past the first head, then count the comparisons from the first
	// head by its pins. The commit's stage pins the head it builds over, so
	// the count starts after it.
	createWatchTestObject(t, ctx, ws, "share/a")
	var pins atomic.Int32
	store := e.writeBlockStore
	setPin(e, func(ctx context.Context, ref *block.BlockRef) (func(), error) {
		if ref.EqualsRef(afterRoot) {
			pins.Add(1)
		}
		return block.PinRoot(ctx, store, ref)
	})

	// Start the watch with a live subscriber, so the watch records no span
	// from the first head.
	reported := make(chan uint64)
	live := headSeqno(t, ctx, e)
	go func() {
		_ = e.WatchChanges(ctx, live, nil, func(seqno uint64, _ *world.ChangeSet) error {
			select {
			case reported <- seqno:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	// Expect a wave of subscribers from the first head to see the commit.
	check := func(changes *world.ChangeSet, want ...string) {
		t.Helper()
		if changes.Unknown {
			t.Fatal("changes since the first head are unknown")
		}
		if got := slices.Sorted(slices.Values(changes.Keys)); !slices.Equal(got, want) {
			t.Fatalf("changed keys %q, want %q", got, want)
		}
	}
	const wave = 8
	windows := make(chan *world.ChangeSet, wave)
	for range wave {
		go func() {
			_ = e.WatchChanges(ctx, after, nil, func(_ uint64, cs *world.ChangeSet) error {
				windows <- cs
				return errWindowRead
			})
		}()
	}
	for range wave {
		check(<-windows, "share/a")
	}

	// Commit again and wait for the watch to record it.
	createWatchTestObject(t, ctx, ws, "share/b")
	want := headSeqno(t, ctx, e)
	for seqno := uint64(0); seqno < want; {
		select {
		case seqno = <-reported:
		case <-time.After(10 * time.Second):
			t.Fatalf("watch did not report seqno %d", want)
		}
	}

	// Expect a late subscriber to see both commits, and the first head to
	// have been compared once.
	_, changes := readChangeWindow(t, ctx, e, after)
	check(changes, "share/a", "share/b")
	if n := pins.Load(); n != 1 {
		t.Fatalf("compared the first head %d times, want 1", n)
	}
}

// TestWatchChangesFilter checks that the change watch hands a filtered
// subscriber only the changes its filter accepts, so a commit it does not
// watch never reaches it.
func TestWatchChangesFilter(t *testing.T) {
	// Keep an unfiltered subscriber that reports each compared seqno.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	after := headSeqno(t, ctx, e)
	reported := make(chan uint64, 16)
	go func() {
		_ = e.WatchChanges(ctx, after, nil, func(seqno uint64, _ *world.ChangeSet) error {
			reported <- seqno
			return nil
		})
	}()

	// Start a subscriber that watches one key.
	type report struct {
		seqno uint64
		keys  []string
	}
	filtered := make(chan report, 16)
	touches := func(cs *world.ChangeSet) bool { return slices.Contains(cs.Keys, "filter/watched") }
	go func() {
		_ = e.WatchChanges(ctx, after, touches, func(seqno uint64, cs *world.ChangeSet) error {
			filtered <- report{seqno, slices.Sorted(slices.Values(cs.Keys))}
			return nil
		})
	}()

	// Wait for both subscriptions so each commit reaches them live.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		n := len(e.indexedSubscribers())
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of 2 subscribers joined", n)
		}
	}

	// Commit an unwatched object and a watched one, each compared before the
	// next.
	var watchedSeqno uint64
	for _, key := range []string{"filter/other", "filter/watched"} {
		createWatchTestObject(t, ctx, ws, key)
		want := headSeqno(t, ctx, e)
		for seqno := uint64(0); seqno < want; {
			select {
			case seqno = <-reported:
			case <-time.After(10 * time.Second):
				t.Fatalf("watch did not report seqno %d", want)
			}
		}
		watchedSeqno = want
	}

	// Expect exactly the watched commit at the filtered subscriber.
	select {
	case got := <-filtered:
		if got.seqno != watchedSeqno || !slices.Equal(got.keys, []string{"filter/watched"}) {
			t.Fatalf("filtered report %d %q, want %d [filter/watched]", got.seqno, got.keys, watchedSeqno)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no report of the watched commit")
	}
	select {
	case got := <-filtered:
		t.Fatalf("extra filtered report %d %q", got.seqno, got.keys)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestWatchChangesEvicted checks that a watcher whose head the Engine no
// longer retains sees unknown changes.
func TestWatchChangesEvicted(t *testing.T) {
	// Watch from the current head, then publish past the retention bound.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	after := headSeqno(t, ctx, e)
	for i := range changeWatchHeads {
		createWatchTestObject(t, ctx, ws, "evict/"+strconv.Itoa(i))
	}

	// Expect unknown changes since the evicted head.
	if _, changes := readChangeWindow(t, ctx, e, after); !changes.Unknown {
		t.Fatalf("changes since an evicted head = %+v, want unknown", changes)
	}
}

// TestWatchChangesCollectedBase checks that a watcher whose head the volume
// collected sees unknown changes rather than a partial list.
func TestWatchChangesCollectedBase(t *testing.T) {
	// Commit an object and watch from that head.
	f := newVolumeFixture(t)
	ctx := t.Context()
	w := fixtureWriter(t, f.engine)
	fixtureObject(t, w, "collected/first", "collected/first")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	after := headSeqno(t, ctx, f.engine)
	base := f.engine.GetRootRef().GetRootRef()

	// Supersede the head and collect it.
	w = fixtureWriter(t, f.engine)
	fixtureObject(t, w, "collected/second", "collected/second")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	if found, err := f.volume.GetBlockExists(ctx, base); err != nil || found {
		t.Fatalf("superseded head retained: %v %v", found, err)
	}

	// Expect unknown changes since the collected head.
	if _, changes := readChangeWindow(t, ctx, f.engine, after); !changes.Unknown {
		t.Fatalf("changes since a collected head = %+v, want unknown", changes)
	}
}

// pinFuncStore pins roots with pin and passes everything else to the store.
type pinFuncStore struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs
	// pin pins a root.
	pin func(ctx context.Context, ref *block.BlockRef) (func(), error)
}

// HoldsRoots reports that the store pins roots.
func (s *pinFuncStore) HoldsRoots() bool { return true }

// PinRoot pins ref with pin.
func (s *pinFuncStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return s.pin(ctx, ref)
}

// setPin replaces how the engine's write store pins roots, keeping its stages
// and proofs.
func setPin(e *Engine, pin func(ctx context.Context, ref *block.BlockRef) (func(), error)) {
	locked := e.bcast.Lock()
	leases, _ := e.writeBlockStore.(block.LeasedRoots)
	proofs, _ := e.writeBlockStore.(block.RootProofs)
	e.writeBlockStore = &pinFuncStore{StoreOps: e.writeBlockStore, LeasedRoots: leases, RootProofs: proofs, pin: pin}
	locked.Unlock()
}

// TestWatchChangesPeerBase checks that a watcher whose base only an absent
// peer could serve sees unknown changes without waiting for the peer.
func TestWatchChangesPeerBase(t *testing.T) {
	// Commit an object and watch from that head.
	f := newVolumeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	w := fixtureWriter(t, f.engine)
	fixtureObject(t, w, "peer/first", "peer/first")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	after := headSeqno(t, ctx, f.engine)
	base := f.engine.GetRootRef().GetRootRef()

	// Supersede the head and leave the base only on an absent peer: a pin
	// that may wait for the peer blocks, and one that may not fails.
	w = fixtureWriter(t, f.engine)
	fixtureObject(t, w, "peer/second", "peer/second")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	store := f.engine.writeBlockStore
	setPin(f.engine, func(ctx context.Context, ref *block.BlockRef) (func(), error) {
		if !ref.EqualsRef(base) {
			return block.PinRoot(ctx, store, ref)
		}
		if block.GetPeerWait(ctx) != nil {
			return nil, block.ErrUnavailable
		}
		<-ctx.Done()
		return nil, context.Cause(ctx)
	})

	// Expect unknown changes since the base before the deadline.
	if _, changes := readChangeWindow(t, ctx, f.engine, after); !changes.Unknown {
		t.Fatalf("changes since a peer-only base = %+v, want unknown", changes)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("watch waited for the absent peer: %v", err)
	}
}

// TestHeadHoldersSharePublicationPin checks that readers and write forks of
// the published head share the pin taken at publication instead of pinning
// again under the publication lock.
func TestHeadHoldersSharePublicationPin(t *testing.T) {
	// Publish a head, then make every later pin block.
	f := newVolumeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	w := fixtureWriter(t, f.engine)
	fixtureObject(t, w, "pin/first", "pin/first")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	setPin(f.engine, func(ctx context.Context, _ *block.BlockRef) (func(), error) {
		<-ctx.Done()
		return nil, context.Cause(ctx)
	})

	// Read the head and open a read snapshot of it.
	err := f.engine.AccessWorldState(ctx, nil, func(*bucket_lookup.Cursor) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	read, err := f.engine.NewBlockEngineTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	read.Discard()

	// Fork the head for writing.
	fork, err := f.engine.ForkBlockTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	fork.Discard()
	if err := ctx.Err(); err != nil {
		t.Fatalf("head holder pinned again: %v", err)
	}
}

// headSeqno returns the seqno of the published head.
func headSeqno(t *testing.T, ctx context.Context, e *Engine) uint64 {
	t.Helper()
	seqno, err := e.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return seqno
}

// readChangeWindow reads the first change window after afterSeqno.
func readChangeWindow(t *testing.T, ctx context.Context, e *Engine, afterSeqno uint64) (uint64, *world.ChangeSet) {
	// Keep the first report and end the watch.
	t.Helper()
	var seqno uint64
	var changes *world.ChangeSet
	err := e.WatchChanges(ctx, afterSeqno, nil, func(s uint64, cs *world.ChangeSet) error {
		seqno, changes = s, cs
		return errWindowRead
	})
	if !errors.Is(err, errWindowRead) {
		t.Fatal(err)
	}
	return seqno, changes
}

// checkQuads checks that got holds exactly the want quads in any order.
func checkQuads(t *testing.T, got []world.GraphQuad, want ...world.GraphQuad) {
	t.Helper()
	format := func(quads []world.GraphQuad) []string {
		out := make([]string, len(quads))
		for i, q := range quads {
			out[i] = q.GetSubject() + " " + q.GetPredicate() + " " + q.GetObj() + " " + q.GetLabel()
		}
		slices.Sort(out)
		return out
	}
	if g, w := format(got), format(want); !slices.Equal(g, w) {
		t.Fatalf("changed quads %q, want %q", g, w)
	}
}

// getFuncStore reads blocks with get and passes everything else to the store.
type getFuncStore struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs
	// get reads a block.
	get func(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error)
}

// GetBlock reads ref with get.
func (s *getFuncStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return s.get(ctx, ref)
}

// TestWatchChangesMissingBlockNoPeer checks that a head comparison reading a
// block only an absent peer could serve reports unknown changes to the
// subscribers instead of waiting for the peer.
func TestWatchChangesMissingBlockNoPeer(t *testing.T) {
	// Commit an object to watch from.
	f := newVolumeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	w := fixtureWriter(t, f.engine)
	fixtureObject(t, w, "hole/first", "hole/first")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	base := f.engine.GetRootRef().GetRootRef()

	// Leave the watched head's root only on an absent peer once armed.
	armed := setPeerOnlyBlock(f.engine, base)

	// Subscribe from the head, then commit past it with the root missing.
	after := headSeqno(t, ctx, f.engine)
	windows := make(chan *world.ChangeSet, 1)
	go func() {
		_ = f.engine.WatchChanges(ctx, after, nil, func(_ uint64, cs *world.ChangeSet) error {
			windows <- cs
			return errWindowRead
		})
	}()
	armed.Store(true)
	w = fixtureWriter(t, f.engine)
	fixtureObject(t, w, "hole/second", "hole/second")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Expect unknown changes before the deadline.
	select {
	case changes := <-windows:
		if !changes.Unknown {
			t.Fatalf("changes across a missing block = %+v, want unknown", changes)
		}
	case <-ctx.Done():
		t.Fatal("change watch waited for the absent peer")
	}
}

// TestWatchChangesCatchUpMissingBlockNoPeer checks that a catch-up comparison
// reading a block only an absent peer could serve reports unknown changes
// instead of waiting for the peer.
func TestWatchChangesCatchUpMissingBlockNoPeer(t *testing.T) {
	// Commit an object to catch up from.
	f := newVolumeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	w := fixtureWriter(t, f.engine)
	fixtureObject(t, w, "hole/first", "hole/first")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	after := headSeqno(t, ctx, f.engine)
	armed := setPeerOnlyBlock(f.engine, f.engine.GetRootRef().GetRootRef())

	// Commit past it and start the watch there, so it records no span from
	// the first head.
	w = fixtureWriter(t, f.engine)
	fixtureObject(t, w, "hole/second", "hole/second")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	live := headSeqno(t, ctx, f.engine)
	go func() {
		_ = f.engine.WatchChanges(ctx, live, nil, func(uint64, *world.ChangeSet) error { return nil })
	}()

	// Expect a subscriber from the first head to see unknown changes before
	// the deadline once its root is missing.
	armed.Store(true)
	_, changes := readChangeWindow(t, ctx, f.engine, after)
	if !changes.Unknown {
		t.Fatalf("changes across a missing block = %+v, want unknown", changes)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("catch-up waited for the absent peer: %v", err)
	}
}

// setPeerOnlyBlock replaces the engine's write store so that, once armed,
// ref is held only by an absent peer: a read that may wait for the peer
// blocks, and one that may not reports ErrUnavailable.
func setPeerOnlyBlock(e *Engine, ref *block.BlockRef) *atomic.Bool {
	// Read ref as an absent peer would serve it once armed.
	armed := &atomic.Bool{}
	store := e.writeBlockStore
	get := func(ctx context.Context, got *block.BlockRef) ([]byte, bool, error) {
		// Read every other block, and ref until armed, from the store.
		if !armed.Load() || !got.EqualsRef(ref) {
			return store.GetBlock(ctx, got)
		}

		// Fail a read that may not wait, and block one that may.
		if block.GetPeerWait(ctx) != nil {
			return nil, false, block.ErrUnavailable
		}
		<-ctx.Done()
		return nil, false, context.Cause(ctx)
	}

	// Wrap the store under the publication lock, keeping its pins, stages and
	// proofs.
	locked := e.bcast.Lock()
	leases, _ := store.(block.LeasedRoots)
	proofs, _ := store.(block.RootProofs)
	e.writeBlockStore = &getFuncStore{StoreOps: store, LeasedRoots: leases, RootProofs: proofs, get: get}
	locked.Unlock()
	return armed
}
