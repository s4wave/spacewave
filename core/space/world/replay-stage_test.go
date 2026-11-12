package space_world

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// localStateSpace is a SharedObject that serves only its local state, from
// one in-memory store.
type localStateSpace struct {
	sobject.SharedObject
	// store is the local state of every store ID.
	store kvtx.Store
}

// AccessLocalStateStore returns the in-memory store.
func (s *localStateSpace) AccessLocalStateStore(context.Context, string, func()) (kvtx.Store, func(), error) {
	return s.store, func() {}, nil
}

// replayFixture is an adapter whose PendingStore keeps at most a bound of
// bytes over an in-memory Volume that frees at once, with its replay stage
// open, as Execute leaves it.
type replayFixture struct {
	t *testing.T
	a *Adapter
	// collect collects the Volume.
	collect func()
}

// newReplayFixture opens a replayFixture whose pages keep at most maxPending
// bytes.
func newReplayFixture(t *testing.T, maxPending int) *replayFixture {
	a, collect := newVolumeAdapter(t, maxPending, time.Nanosecond)
	a.so = &localStateSpace{store: kvtx_inmem.NewStore()}
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{}}}
	stage, err := newReplayStage(t.Context(), a.pages)
	if err != nil {
		t.Fatal(err)
	}
	a.replay.stage = stage
	t.Cleanup(stage.end)
	return &replayFixture{t: t, a: a, collect: collect}
}

// entry returns a page named name, padded to at least size bytes, that names
// children.
func (f *replayFixture) entry(name string, size int, children ...*block.BlockRef) *block.PutBatchEntry {
	f.t.Helper()
	data := []byte("replay " + name)
	for _, c := range children {
		data = append(data, c.MarshalString()...)
	}
	if len(data) < size {
		data = append(data, bytes.Repeat([]byte{'.'}, size-len(data))...)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true}
}

// put stores a page of at least size bytes through the replay.
func (f *replayFixture) put(name string, size int, children ...*block.BlockRef) *block.BlockRef {
	f.t.Helper()
	e := f.entry(name, size, children...)
	if _, err := f.a.replay.store().PutBlockBatch(f.t.Context(), []*block.PutBatchEntry{e}); err != nil {
		f.t.Fatal(err)
	}
	return e.Ref
}

// apply records a position whose World is world.
func (f *replayFixture) apply(name string, world *block.BlockRef) {
	f.a.replay.positions = append(f.a.replay.positions, spacePosition{
		op:    &sobject.OrderedOperation{Hash: []byte(name)},
		state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: world}},
	})
}

// found reports whether the pages serve ref.
func (f *replayFixture) found(ref *block.BlockRef) bool {
	f.t.Helper()
	_, ok, err := f.a.pages.GetBlock(f.t.Context(), ref)
	if err != nil {
		f.t.Fatal(err)
	}
	return ok
}

// requireFound fails unless the pages serve every ref.
func (f *replayFixture) requireFound(what string, refs ...*block.BlockRef) {
	f.t.Helper()
	for _, ref := range refs {
		if !f.found(ref) {
			f.t.Fatalf("a collection freed a page of %s: %v", what, ref)
		}
	}
}

// save saves the cursor and waits for the save to land.
func (f *replayFixture) save() {
	f.t.Helper()
	if err := f.a.saveCursor(f.t.Context()); err != nil {
		f.t.Fatal(err)
	}
	f.a.waitBackground()
}

// TestReplayStageHoldsUnsavedWorlds checks, on a Volume that frees at once
// and an adapter that keeps no page in memory, that every page replay stores
// survives collections until a cursor save that names its World lands: before
// any save, and while a save due during another one is skipped, as after a
// remount whose replay stores again the pages an earlier run stored. Once a
// later save lands, a page only an unsaved World named is freed. It fails,
// whatever the timing, when replay writes to the PendingStore directly.
func TestReplayStageHoldsUnsavedWorlds(t *testing.T) {
	f := newReplayFixture(t, 0)
	a, ctx := f.a, t.Context()

	// A World replay stored survives a collection before any save.
	dead := f.put("dead page", 0)
	first := f.put("first World", 0, dead)
	f.apply("first", first)
	f.collect()
	f.requireFound("an unsaved World", first, dead)

	// A save due while another runs is skipped, and its World survives.
	a.cursorSaving.Store(true)
	page := f.put("shared page", 0)
	second := f.put("second World", 0, page)
	f.apply("second", second)
	if err := a.maybeSaveCursor(ctx); err != nil {
		t.Fatal(err)
	}
	f.collect()
	f.requireFound("a World whose save was skipped", first, dead, second, page)
	a.cursorSaving.Store(false)

	// A save of the second World lands; a World applied while it ran stays
	// held by the next epoch.
	if err := a.saveCursor(ctx); err != nil {
		t.Fatal(err)
	}
	third := f.put("third World", 0, page)
	f.apply("third", third)
	a.waitBackground()
	f.collect()
	f.requireFound("the saved World", second, page)
	f.requireFound("a World applied during a save", third)

	// The next save lands too, and its root move releases the first epoch:
	// the pages only the unsaved first World named are freed.
	f.save()
	f.collect()
	f.requireFound("a saved World", second, third, page)
	if f.found(first) || f.found(dead) {
		t.Fatal("a landed save left held the pages only an unsaved World named")
	}
}

// TestReplayStageHoldsEarlyDrains checks that a replayed transaction's early
// drain survives its transaction's end until a save of a World that names it
// lands. The transaction writes as the World engine's buildWriteState and
// commit do: through a buffer over a stage the replay store opens, whose
// commit batch skips the drained page. A drain past the bound that went to a
// stage of its own is freed once the transaction releases that stage, while
// the unsaved World still names it.
func TestReplayStageHoldsEarlyDrains(t *testing.T) {
	f := newReplayFixture(t, 1000)
	ctx := t.Context()

	// Fill most of the bound with a kept page.
	f.put("kept page", 600)

	// Open the transaction's stage and buffer as buildWriteState does.
	staged, releaseStage, err := block.OpenStage(ctx, f.a.replay.store())
	if err != nil {
		t.Fatal(err)
	}
	writes := block.NewBufferedStoreWithSettings(ctx, staged, &block.BufferedStoreSettings{RecordWrites: true})

	// Write a page past the bound and drain it early, then the World that
	// names it.
	drained := f.entry("drained page", 500)
	if _, err := writes.PutBlockBatch(ctx, []*block.PutBatchEntry{drained}); err != nil {
		t.Fatal(err)
	}
	if err := writes.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	world := f.entry("World", 0, drained.Ref)
	if _, err := writes.PutBlockBatch(ctx, []*block.PutBatchEntry{world}); err != nil {
		t.Fatal(err)
	}

	// Commit as the engine does: the batch holds only the World, which goes
	// to the replay store, then the transaction ends.
	batch, err := writes.TakeReachable(ctx, world.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Entries) != 1 {
		t.Fatalf("commit batch has %d entries, want the World alone", len(batch.Entries))
	}
	_, err = f.a.replay.store().PutBlockBatch(ctx, batch.Entries)
	batch.Complete(err)
	if err != nil {
		t.Fatal(err)
	}
	if err := writes.ReleaseUnreached(ctx, world.Ref); err != nil {
		t.Fatal(err)
	}
	releaseStage()
	f.apply("World", world.Ref)

	// The drained page survives until a save of its World lands, and after.
	f.collect()
	f.requireFound("an unsaved World", world.Ref, drained.Ref)
	f.save()
	f.collect()
	f.requireFound("a saved World", world.Ref, drained.Ref)
}

// TestReplayStageFloorCoversReleasedEpoch checks that a save that is not
// full, landing after a full save was skipped, leaves no World at or above
// the floor naming a page it freed. The skipped full save moves the floor
// below a World whose page went past the bound to the current epoch; the
// later save ends that epoch, and the World above the floor then names a
// freed page unless the floor moves past it. An acknowledgment a reorder puts
// in place of the later World takes the World before it, so the floor must
// cover it too.
func TestReplayStageFloorCoversReleasedEpoch(t *testing.T) {
	f := newReplayFixture(t, 1000)
	a, ctx := f.a, t.Context()

	// Apply a first World, then one whose large page goes past the bound to
	// the current epoch and whose other page fills the bound past half.
	f.apply("zero", f.put("zero World", 0))
	stored := f.put("stored page", 1200)
	kept := f.put("kept page", 500)
	world := f.put("full World", 0, stored, kept)
	f.apply("full", world)
	if !a.pages.Full() {
		t.Fatal("pages are not full")
	}

	// A full save due while another runs is skipped, but the floor moves to
	// the last World and the sweep drops the others.
	a.cursorSaving.Store(true)
	if err := a.maybeSaveCursor(ctx); err != nil {
		t.Fatal(err)
	}
	a.cursorSaving.Store(false)
	full := len(a.replay.positions) - 1
	if a.replay.floor != full {
		t.Fatalf("floor after the skipped save = %d, want %d", a.replay.floor, full)
	}

	// A later save of another World lands and ends the epoch.
	f.apply("later", f.put("later World", 0))
	f.save()
	f.collect()

	// Replay computes the full World again, or still finds its pages.
	if a.replay.floor <= full {
		f.requireFound("a World above the floor", world, stored, kept)
	}

	// A reorder puts an acknowledgment in place of the later World, which
	// takes the full World as its own: it shares that World's floor.
	r := a.replay
	r.positions = r.positions[:len(r.positions)-1]
	pos, err := r.run(ctx, len(r.positions), &sobject.OrderedOperation{Hash: []byte("ack")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.positions = append(r.positions, pos)
	if r.floor < len(r.positions) {
		f.requireFound("the acknowledgment's World", world, stored, kept)
	}
}

// TestCursorSaveHoldsServedWorlds checks that a cursor save which ends an
// epoch that stored a page holds the Worlds the adapter still serves that are
// no position: the installed head after a reorder cut the positions below
// it, and a World an engine reader still reads after its head retired. The
// save names a World that does not reach the page, so a collection after it
// lands frees the page unless the save holds those Worlds too.
func TestCursorSaveHoldsServedWorlds(t *testing.T) {
	for _, served := range []string{"installed head", "reader"} {
		t.Run(served, func(t *testing.T) {
			// Replay and serve a World whose page goes past the bound to
			// the current epoch.
			f := newReplayFixture(t, 1000)
			a := f.a
			f.apply("zero", f.put("zero World", 0))
			stored := f.put("stored page", 1200)
			world := f.put("served World", 0, stored)
			f.apply("served", world)
			if served == "reader" {
				a.readers = readRoots{world}
			} else {
				a.head.Root = world
			}

			// A reorder cuts the positions below it, and the save of the
			// World in its place ends the epoch.
			a.replay.positions = a.replay.positions[:1]
			f.apply("reordered", f.put("reordered World", 0))
			f.save()
			f.collect()
			f.requireFound("the "+served+"'s World", world, stored)
		})
	}
}

// peerSpace is a localStateSpace that names its peer.
type peerSpace struct {
	*localStateSpace
	// id is the peer ID.
	id peer.ID
}

// GetPeerID returns the peer ID.
func (s *peerSpace) GetPeerID() peer.ID {
	return s.id
}

// TestAdoptAfterEpochEnds checks, through Apply, that a candidate
// built on a replayed World whose page only a replay epoch stored is not
// adopted once a later save ended that epoch and a reorder puts its
// operation back after that World: the candidate may share a page only the
// ended epoch held, so the operation runs again on the World computed again.
func TestAdoptAfterEpochEnds(t *testing.T) {
	f := newReplayFixture(t, 1000)
	a, ctx := f.a, t.Context()
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	a.so = &peerSpace{localStateSpace: a.so.(*localStateSpace), id: p.GetPeerID()}
	a.missing = make(map[string]struct{})

	// Replay a World whose page goes past the bound to the current epoch.
	f.apply("zero", f.put("zero World", 0))
	stored := f.put("stored page", 1200)
	base := f.put("base World", 0, stored)
	f.apply("base", base)
	if a.pages.Full() {
		t.Fatal("pages are full")
	}

	// A local writer admits a small candidate on that World that keeps the
	// stored page.
	cand := f.entry("candidate World", 0, stored)
	adm := a.newAdmission(world.Candidate{Base: base, Root: cand.Ref, Entries: []*block.PutBatchEntry{cand}}, nil, nil)
	a.admit(adm)
	if err := a.storeAdmitted(ctx, adm); err != nil {
		t.Fatal(err)
	}
	adm.hash = []byte("local")

	// Another device's operation lands first; its World does not reach the
	// page, and its save ends the epoch.
	f.apply("later", f.put("later World", 0))
	f.save()
	f.collect()

	// A reorder puts this device's operation after the base World.
	data, err := (&WorldOp{Body: &WorldOp_SetRetainedRoot{SetRetainedRoot: &SetRetainedRootOp{}}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	op := &sobject.OrderedOperation{
		Hash:  adm.hash,
		Inner: &sobject.SOOperationInner{PeerId: p.GetPeerID().String()},
		Data:  data,
	}
	if _, err := a.Apply(ctx, 2, op); err != nil {
		t.Fatal(err)
	}

	// The position does not take the candidate's World.
	if refEqual(a.replay.positions[2].state.GetHeadRef().GetRootRef(), cand.Ref) {
		t.Fatal("adopted a candidate built on a World below the floor")
	}
}

// TestCursorSaveRefusalDamages checks that a cursor save the PendingStore
// refuses, since the saved World names a page no store holds, is tried once
// more, then, refused again, damages the adapter, which then starts no save
// until the refusal is due again; that a save after the page is restored
// clears the damage; and that a refusal healed before the second save leaves
// no damage.
func TestCursorSaveRefusalDamages(t *testing.T) {
	for _, healed := range []bool{false, true} {
		t.Run(map[bool]string{false: "forever", true: "healed"}[healed], func(t *testing.T) {
			f := newReplayFixture(t, 1<<20)
			a := f.a

			// The head World names a page no store holds.
			missing := f.entry("missing page", 0)
			f.apply("head", f.put("head World", 0, missing.Ref))
			f.save()
			if a.Damaged() != nil {
				t.Fatalf("one refused save damaged the adapter: %v", a.Damaged())
			}
			if healed {
				f.put("missing page", 0)
			}
			f.save()
			if healed {
				if a.Damaged() != nil {
					t.Fatalf("a healed save left damage: %v", a.Damaged())
				}
				return
			}
			if damage := a.Damaged(); damage.GetRefusals() != 2 || !damage.GetChild().EqualVT(missing.Ref) {
				t.Fatalf("damage after two refused saves = %v", a.Damaged())
			}

			// Once damaged, a due save does not start.
			r := a.replay
			for range replayCursorOperations {
				f.apply("more", r.positions[0].state.GetHeadRef().GetRootRef())
			}
			saved := r.saved
			if err := a.maybeSaveCursor(t.Context()); err != nil {
				t.Fatal(err)
			}
			if r.saved != saved {
				t.Fatalf("a damaged adapter saved the cursor at %d", r.saved)
			}

			// The page is restored; the save due after the retry delay lands
			// and clears the damage.
			f.put("missing page", 0)
			a.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
			if err := a.maybeSaveCursor(t.Context()); err != nil {
				t.Fatal(err)
			}
			a.waitBackground()
			if r.saved == saved || a.Damaged() != nil {
				t.Fatalf("saved at %d, damage %v after the page was restored", r.saved, a.Damaged())
			}
		})
	}
}

// TestCursorSavePayloadRefusalDamages checks that a cursor save whose payload
// hold the store refuses counts as a refused save rather than stopping replay:
// two such saves in a row damage the adapter, naming the missing child.
func TestCursorSavePayloadRefusalDamages(t *testing.T) {
	f := newReplayFixture(t, 1<<20)
	a := f.a

	// The head position's payload names a page no store holds.
	missing := f.entry("missing payload page", 0)
	f.apply("head", f.put("head World", 0))
	a.replay.positions[0].payloads = []*block.BlockRef{f.put("payload", 0, missing.Ref)}
	f.save()
	f.save()
	if damage := a.Damaged(); damage.GetRefusals() != 2 || !damage.GetChild().EqualVT(missing.Ref) {
		t.Fatalf("damage after two refused payload holds = %v", damage)
	}
}

// TestTrimRefusalDamages checks that a trim the store refuses, here a base
// World naming a page no store holds, moves nothing and, refused again on the
// retry, damages the adapter instead of only logging.
func TestTrimRefusalDamages(t *testing.T) {
	f := newReplayFixture(t, 1<<20)
	a := f.a
	missing := f.entry("missing base page", 0)
	plan := &trimPlan{base: f.put("trim base", 0, missing.Ref), live: map[string]struct{}{}, held: map[string]struct{}{}}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.runTrimNewest(ctx, plan)
	}()
	for deadline := time.Now().Add(5 * time.Second); a.Damaged() == nil; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("a refused trim did not damage the adapter")
		}
	}
	cancel()
	<-done
	if damage := a.Damaged(); !damage.GetChild().EqualVT(missing.Ref) || refEqual(a.held[RootReplayBase], plan.base) {
		t.Fatalf("damage %v, base moved %v", damage, refEqual(a.held[RootReplayBase], plan.base))
	}
}

// TestPendingCandidateHoldsEpochPage checks that a local candidate built on a
// replayed World whose page only a replay epoch stored keeps that page while
// it is pending: another device's operation and a cursor save end the epoch,
// and a collection must not free the page the writer's head still reaches.
func TestPendingCandidateHoldsEpochPage(t *testing.T) {
	f := newReplayFixture(t, 1000)
	a, ctx := f.a, t.Context()

	// Replay a World whose page goes past the bound to the current epoch.
	f.apply("zero", f.put("zero World", 0))
	stored := f.put("stored page", 1200)
	base := f.put("base World", 0, stored)
	f.apply("base", base)
	if a.pages.Full() {
		t.Fatal("pages are full")
	}

	// A local writer admits a candidate on that World that keeps the page.
	cand := f.entry("candidate World", 0, stored)
	adm := a.newAdmission(world.Candidate{Base: base, Root: cand.Ref, Entries: []*block.PutBatchEntry{cand}}, nil, nil)
	a.admit(adm)
	if err := a.storeAdmitted(ctx, adm); err != nil {
		t.Fatal(err)
	}

	// Another device's operation lands, and its save ends the epoch.
	f.apply("later", f.put("later World", 0))
	f.save()
	f.collect()
	f.requireFound("the pending candidate's World", stored)
}
