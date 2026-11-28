package world_block

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
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

// TestWatchChangesCatchUpKeepsIntermediateDeletion checks a subscriber that
// joins after both commits rather than receiving a live span.
func TestWatchChangesCatchUpKeepsIntermediateDeletion(t *testing.T) {
	// Create and delete an object before the first subscriber joins.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	after := headSeqno(t, ctx, e)
	const key = "catch-up/transient"
	createWatchTestObject(t, ctx, ws, key)
	if deleted, err := ws.DeleteObject(ctx, key); err != nil || !deleted {
		t.Fatalf("deleted = %t, err %v", deleted, err)
	}

	// The retained history contains the key even though neither endpoint does.
	seqno, changes := readChangeWindow(t, ctx, e, after)
	if seqno != headSeqno(t, ctx, e) || changes.Unknown || !slices.Equal(changes.Keys, []string{key}) {
		t.Fatalf("catch-up at %d gave %+v, want %q", seqno, changes, key)
	}
}

// TestWatchChangesImportedRoot checks that a head installed from outside the
// Engine, as a SharedObject replay or a snapshot import installs it, yields
// Unknown when it carries no summary, without depending on either tree format.
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

			// An imported head carries no process-local transition evidence.
			_, changes := readChangeWindow(t, ctx, e, after)
			if !changes.Unknown {
				t.Fatal("import without a summary was certified")
			}
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
// the change watch at the same head share one comparison when a publication
// arrived without a recorded ChangeSet, and that a later one continues from
// that comparison through the spans the watch recorded since.
func TestWatchChangesShareCatchUp(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Commit past the first head through a second engine on the same Authority
	// so f.engine receives no exact summary. Count any attempt to pin the old
	// head: reconnecting subscribers must receive Unknown without storage I/O.
	f := newVolumeFixture(t)
	e := f.engine
	ws := world.NewEngineWorldState(e, true)
	createWatchTestObject(t, ctx, ws, "share/first")
	after := headSeqno(t, ctx, e)
	afterRoot := e.GetRootRef().GetRootRef()
	commitExternalObject(t, ctx, f, "share/a")
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

	// Every reconnect must refresh when the imported transition lacks evidence.
	check := func(changes *world.ChangeSet) {
		t.Helper()
		if !changes.Unknown {
			t.Fatal("missing transition evidence was certified")
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
		check(<-windows)
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

	// Later reconnects also refresh without pinning the obsolete head.
	_, changes := readChangeWindow(t, ctx, e, after)
	check(changes)
	if n := pins.Load(); n != 0 {
		t.Fatalf("pinned the first head %d times, want 0", n)
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
// collected sees unknown changes when an intervening publication arrived
// without a recorded ChangeSet, rather than a partial list.
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

	// Supersede the head through another engine so f.engine records no
	// ChangeSet for it, then collect the superseded head.
	commitExternalObject(t, ctx, f, "collected/second")
	f.collect(t)
	if found, err := f.volume.GetBlockExists(ctx, base); err != nil || found {
		t.Fatalf("superseded head retained: %v %v", found, err)
	}

	// Expect unknown changes since the collected head.
	if _, changes := readChangeWindow(t, ctx, f.engine, after); !changes.Unknown {
		t.Fatalf("changes since a collected head = %+v, want unknown", changes)
	}
}

// TestWatchChangesCatchUpAcrossCollectedHeads checks that a subscriber resuming
// after every past head has been collected still receives an exact ChangeSet
// from local commits, including changes that cancel out and spans longer than
// ten publications.
func TestWatchChangesCatchUpAcrossCollectedHeads(t *testing.T) {
	// Commit a starting head to resume from.
	f := newVolumeFixture(t)
	ctx := t.Context()
	commitLocal := func(fn func(w *EngineTx)) {
		t.Helper()
		w := fixtureWriter(t, f.engine)
		fn(w)
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	commitLocal(func(w *EngineTx) {
		fixtureObject(t, w, "trim/base", "trim/base")
	})
	after := headSeqno(t, ctx, f.engine)
	pastRoots := []*block.BlockRef{f.engine.GetRootRef().GetRootRef().Clone()}

	// Commit ten local writes, including an object
	// created and deleted across commits and a quad set and deleted in one
	// commit.
	commitLocal(func(w *EngineTx) {
		fixtureObject(t, w, "trim/kept", "trim/kept")
	})
	pastRoots = append(pastRoots, f.engine.GetRootRef().GetRootRef().Clone())
	commitLocal(func(w *EngineTx) {
		fixtureObject(t, w, "trim/transient", "trim/transient")
	})
	pastRoots = append(pastRoots, f.engine.GetRootRef().GetRootRef().Clone())
	commitLocal(func(w *EngineTx) {
		if deleted, err := w.DeleteObject(ctx, "trim/transient"); err != nil || !deleted {
			t.Fatalf("delete transient: deleted=%t err=%v", deleted, err)
		}
	})
	pastRoots = append(pastRoots, f.engine.GetRootRef().GetRootRef().Clone())
	cancelledQuad := world.NewGraphQuadWithKeys("trim/kept", "<temp>", "trim/base", "")
	commitLocal(func(w *EngineTx) {
		if err := w.SetGraphQuad(ctx, cancelledQuad); err != nil {
			t.Fatal(err)
		}
		if err := w.DeleteGraphQuad(ctx, cancelledQuad); err != nil {
			t.Fatal(err)
		}
	})
	pastRoots = append(pastRoots, f.engine.GetRootRef().GetRootRef().Clone())
	wantKeys := []string{"trim/kept", "trim/transient"}
	for i := range 10 {
		key := "trim/step-" + strconv.Itoa(i)
		wantKeys = append(wantKeys, key)
		commitLocal(func(w *EngineTx) {
			fixtureObject(t, w, key, key)
		})
		if i < 9 {
			pastRoots = append(pastRoots, f.engine.GetRootRef().GetRootRef().Clone())
		}
	}

	// Publication cleanup may still hold a past head after a durable receipt.
	if err := f.engine.WaitIdle(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify ReadRoots reports none of the past heads, then collect them all.
	readRoots := f.engine.ReadRoots()
	for _, past := range pastRoots {
		if slices.ContainsFunc(readRoots, func(r *block.BlockRef) bool { return r.EqualsRef(past) }) {
			t.Fatalf("ReadRoots still reports past head %v", past)
		}
	}
	f.collect(t)
	for _, past := range pastRoots {
		if found, err := f.volume.GetBlockExists(ctx, past); err != nil || found {
			t.Fatalf("past head %v still present after collection: found=%t err=%v", past, found, err)
		}
	}

	// Resume from the collected starting head and expect the exact ChangeSet.
	seqno, changes := readChangeWindow(t, ctx, f.engine, after)
	if seqno != headSeqno(t, ctx, f.engine) {
		t.Fatalf("catch-up seqno = %d, want %d", seqno, headSeqno(t, ctx, f.engine))
	}
	if changes.Unknown {
		t.Fatal("changes across collected local commits are unknown")
	}
	slices.Sort(wantKeys)
	if got := slices.Sorted(slices.Values(changes.Keys)); !slices.Equal(got, wantKeys) {
		t.Fatalf("changed keys %q, want %q", got, wantKeys)
	}
	checkQuads(t, changes.Quads, cancelledQuad)
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

	// Supersede the head through another engine so f.engine records no
	// ChangeSet for it, and leave the base only on an absent peer: a pin that
	// may wait for the peer blocks, and one that may not fails.
	commitExternalObject(t, ctx, f, "peer/second")
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

// commitExternalObject commits one object through a second engine sharing f's
// Authority and waits until f.engine installs the resulting head via
// followHead, so f.engine records the publication without a ChangeSet.
func commitExternalObject(t *testing.T, ctx context.Context, f *volumeFixture, key string) {
	// Commit on a peer engine sharing the Authority.
	t.Helper()
	peer := f.openEngine(t, f.authority())
	w := fixtureWriter(t, peer)
	fixtureObject(t, w, key, key)
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wantRoot := peer.GetRootRef().GetRootRef()

	// Wait for f.engine's head watch to install the peer's root.
	for {
		if f.engine.GetRootRef().GetRootRef().EqualsRef(wantRoot) {
			return
		}
		wait := f.engine.headSignal.Load().changed
		select {
		case <-ctx.Done():
			t.Fatalf("engine did not follow external head %v: %v", wantRoot, ctx.Err())
		case <-wait:
		}
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

// TestCloseEndsFollowerMissingBlockRead checks that Close returns while the
// head follower waits for a block only an absent peer could serve.
func TestCloseEndsFollowerMissingBlockRead(t *testing.T) {
	// Commit twice, so the first head's root is no longer the published one.
	f := newVolumeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	w := fixtureWriter(t, f.engine)
	fixtureObject(t, w, "hole/first", "hole/first")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	first := f.engine.GetRootRef().GetRootRef().Clone()
	w = fixtureWriter(t, f.engine)
	fixtureObject(t, w, "hole/second", "hole/second")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Report the first root as a newer head with it only on an absent peer.
	report := world.Head{Root: first, Seqno: headSeqno(t, ctx, f.engine) + 1, Durable: true}
	armed, waiting := setPeerOnlyBlock(t, f.engine, first)
	armed.Store(true)
	followed := make(chan struct{})
	go func() {
		defer close(followed)
		f.engine.followHead(report)
	}()
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal("follower never read the missing root")
	}

	// Close must end the follower's read.
	closed := make(chan error, 1)
	go func() { closed <- f.engine.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Close waited for the follower's missing block read")
	}
	<-followed
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
	armed, _ := setPeerOnlyBlock(t, f.engine, base)

	// Subscribe from the head, then publish past it from another engine with
	// the base root missing so the watch falls back to comparing trees.
	after := headSeqno(t, ctx, f.engine)
	windows := make(chan *world.ChangeSet, 1)
	go func() {
		_ = f.engine.WatchChanges(ctx, after, nil, func(_ uint64, cs *world.ChangeSet) error {
			windows <- cs
			return errWindowRead
		})
	}()
	for deadline := time.Now().Add(5 * time.Second); len(f.engine.indexedSubscribers()) != 1; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("subscriber did not join")
		}
	}
	armed.Store(true)
	commitExternalObject(t, ctx, f, "hole/second")

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
	armed, _ := setPeerOnlyBlock(t, f.engine, f.engine.GetRootRef().GetRootRef())

	// Publish past it through another engine so f.engine records no ChangeSet,
	// and start the watch there, so it records no span from the first head.
	commitExternalObject(t, ctx, f, "hole/second")
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
// blocks until its context ends, and one that may not reports ErrUnavailable.
// waiting closes when the first read starts blocking.
func setPeerOnlyBlock(t *testing.T, e *Engine, ref *block.BlockRef) (armed *atomic.Bool, waiting <-chan struct{}) {
	// Join the Authority watch before replacing an engine-lifetime store.
	// These fixtures have no subscribers or concurrent callers yet.
	t.Helper()
	if e.headWatchRelease != nil {
		e.headWatchRelease()
		e.headWatchRelease = nil
	}
	if err := e.WaitIdle(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Read ref as an absent peer would serve it once armed.
	armed = &atomic.Bool{}
	blocked := make(chan struct{})
	var blockedOnce sync.Once
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
		blockedOnce.Do(func() { close(blocked) })
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

	// Start the Authority watch with the immutable replacement already installed.
	if e.authority != nil {
		release, err := e.authority.WatchHead(e.closing, e.followHead)
		if err != nil {
			t.Fatal(err)
		}
		e.headWatchRelease = release
	}
	return armed, blocked
}

// TestWatchIndexedChangesCovered checks the seqno a slotted call reports as
// covered when its context ends. A call whose interest skipped many commits
// covers through the last head the watch handed out, so the next call starts
// there with no unknown change although the skipped commits outnumber the
// recorded heads. A member committed while the subscriber idles in the slot,
// after the covered seqno and before the next call references it, arrives in
// that call exactly once.
func TestWatchIndexedChangesCovered(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	e := newRetirementTestEngine(t, ctx, nil)
	ws := world.NewEngineWorldState(e, true)
	slot := new(world.WatchSlot)
	defer slot.Close()

	// call runs one slotted WatchIndexedChanges after afterSeqno for keys,
	// reporting each change, until stop cancels it and returns the slot's
	// covered seqno.
	type report struct {
		seqno   uint64
		keys    []string
		unknown bool
	}
	call := func(afterSeqno uint64, keys ...string) (<-chan report, func() uint64) {
		t.Helper()
		interest, err := world.NewNamedInterest(keys, nil)
		if err != nil {
			t.Fatal(err)
		}
		callCtx, callCancel := context.WithCancel(world.WithWatchSlot(ctx, slot))
		reports := make(chan report, 16)
		done := make(chan error, 1)
		go func() {
			done <- e.WatchIndexedChanges(callCtx, afterSeqno, interest, func(seqno uint64, cs *world.ChangeSet) error {
				reports <- report{seqno, slices.Sorted(slices.Values(cs.Keys)), cs.Unknown}
				return nil
			})
		}()
		return reports, func() uint64 {
			callCancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("call ended with %v, want canceled", err)
			}
			return slot.Covered()
		}
	}
	expect := func(reports <-chan report, seqno uint64, key string) {
		t.Helper()
		select {
		case got := <-reports:
			if got.unknown || got.seqno != seqno || !slices.Equal(got.keys, []string{key}) {
				t.Fatalf("report %+v, want %q at %d", got, key, seqno)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("no report of %q at %d", key, seqno)
		}
	}

	// Follow the old key and receive its commit.
	reports, stop := call(headSeqno(t, ctx, e), "covered/old")
	for len(e.indexedSubscribers()) != 1 {
		time.Sleep(time.Millisecond)
	}
	createWatchTestObject(t, ctx, ws, "covered/old")
	old := headSeqno(t, ctx, e)
	expect(reports, old, "covered/old")

	// Commit past the recorded heads without referencing the old key, and
	// wait until the watch handed out the last head.
	for i := range changeWatchHeads + 2 {
		createWatchTestObject(t, ctx, ws, "covered/skip-"+strconv.Itoa(i))
	}
	head := headSeqno(t, ctx, e)
	locked := e.changeBcast.Lock()
	w := e.changeWatch
	locked.Unlock()
	for deadline := time.Now().Add(10 * time.Second); w.handedSeqno() != head; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("watch handed out %d, want %d", w.handedSeqno(), head)
		}
	}
	if covered := stop(); covered != head {
		t.Fatalf("covered %d, want the head %d past the last report %d", covered, head, old)
	}

	// The member commits while the subscriber idles. The next call from the
	// covered head references the member and receives its commit once, then
	// the next commit.
	createWatchTestObject(t, ctx, ws, "covered/member")
	member := headSeqno(t, ctx, e)
	reports, stop = call(head, "covered/old", "covered/member")
	expect(reports, member, "covered/member")
	if _, err := ws.DeleteObject(ctx, "covered/old"); err != nil {
		t.Fatal(err)
	}
	expect(reports, headSeqno(t, ctx, e), "covered/old")
	if covered := stop(); covered != headSeqno(t, ctx, e) {
		t.Fatalf("covered %d after the last report, want %d", covered, headSeqno(t, ctx, e))
	}
}

// BenchmarkChangeFeedCommit measures complete World commits with 3,000 live
// subscriptions and 256 object names per transition on a real s4db Volume.
func BenchmarkChangeFeedCommit(b *testing.B) {
	// Keep subscribers alive for the timed commit stream.
	ctx, cancel := context.WithCancel(context.Background())
	e := newVolumeFixture(b, WithReaderPinDelay(defaultReaderPinDelay)).engine
	var watchers sync.WaitGroup
	for range 3000 {
		watchers.Go(func() {
			_ = e.WatchChanges(ctx, 0, nil, func(uint64, *world.ChangeSet) error { return nil })
		})
	}
	defer func() { cancel(); watchers.Wait() }()

	// Wait until the workload has all 3,000 indexed subscribers.
	for len(e.indexedSubscribers()) != 3000 {
		time.Sleep(time.Millisecond)
	}

	// Commit 256 creates per transaction, including the real block-store path.
	var commitTime time.Duration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := e.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		for j := range 256 {
			key := "feed/" + strconv.Itoa(i) + "/" + strconv.Itoa(j)
			obj, _, err := world.CreateWorldObject(ctx, tx, key, func(c *block.Cursor) error {
				c.SetBlock(block_mock.NewExample(key), true)
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
			world.ReleaseObjectState(obj)
		}
		start := time.Now()
		if err := tx.Commit(ctx); err != nil {
			b.Fatal(err)
		}
		commitTime += time.Since(start)
	}
	b.StopTimer()
	b.ReportMetric(float64(commitTime.Nanoseconds())/float64(b.N), "commit-ns/op")
}

// TestWatchChangesRacingStartsAndLag checks coverage and order while writes
// overrun the notice ring, watch starts race them, and one callback stays behind.
func TestWatchChangesRacingStartsAndLag(t *testing.T) {
	// Start a subscriber whose first callback can be held across ring eviction.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	e := newRetirementTestEngine(t, ctx, nil)
	const commits = changeWatchHeads + 12
	first := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan error, 9)
	watch := func(after uint64, lag bool) {
		done <- e.WatchChanges(ctx, after, nil, func(seqno uint64, changes *world.ChangeSet) error {
			// Require each report to cover its full interval and no future write.
			if seqno <= after || seqno > commits {
				return errors.New("unordered or invented revision")
			}
			if !changes.Unknown {
				want := make([]string, 0, seqno-after)
				for i := after + 1; i <= seqno; i++ {
					want = append(want, "race/"+strconv.FormatUint(i, 10))
				}
				slices.Sort(want)
				if !slices.Equal(changes.Keys, want) || len(changes.Quads) != 0 {
					return errors.New("missed or invented change in a certified interval")
				}
			}
			after = seqno

			// Hold only the subscriber, leaving the worker and every writer free.
			if lag {
				lag = false
				close(first)
				select {
				case <-resume:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			if seqno == commits {
				return errWindowRead
			}
			return nil
		})
	}
	go watch(0, true)
	ws := world.NewEngineWorldState(e, true)
	createWatchTestObject(t, ctx, ws, "race/1")
	select {
	case <-first:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Stall feed bookkeeping itself while new watches and real writes race.
	locked := e.changeBcast.Lock()
	for range 8 {
		go watch(1, false)
	}
	written := make(chan struct{})
	go func() {
		defer close(written)
		for i := 2; i <= commits; i++ {
			createWatchTestObject(t, ctx, ws, "race/"+strconv.Itoa(i))
		}
	}()
	select {
	case <-written:
	case <-ctx.Done():
		locked.Unlock()
		t.Fatal("World writes waited behind feed bookkeeping")
	}
	locked.Unlock()
	close(resume)

	// Every subscriber must reach the final revision, with exact or Unknown coverage.
	for range 9 {
		select {
		case err := <-done:
			if !errors.Is(err, errWindowRead) {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("subscriber missed the final change")
		}
	}
}

// TestWatchChangesUnconfirmedSummary rejects touched names from an unaccepted
// candidate even when its proposed predecessor and result match a publication.
func TestWatchChangesUnconfirmedSummary(t *testing.T) {
	// Publish a real write, then model an external notice without its summary.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx, nil)
	before := e.GetRootRef()
	afterSeqno := headSeqno(t, ctx, e)
	ws := world.NewEngineWorldState(e, true)
	createWatchTestObject(t, ctx, ws, "proof/actual")
	notice := *e.latestNotice.Load()
	notice.changes = nil
	e.watchedHeads[notice.publication%changeWatchHeads].Store(&notice)

	// A failed candidate's matching roots do not authorize invented names.
	e.registerPendingChange(before.GetRootRef(), notice.ref.GetRootRef(), &changeSummary{
		keys: map[string]struct{}{"proof/invented": {}},
	})
	_, changes := readChangeWindow(t, ctx, e, afterSeqno)
	if !changes.Unknown || len(changes.Keys) != 0 {
		t.Fatalf("unconfirmed summary was used: %+v", changes)
	}

}
