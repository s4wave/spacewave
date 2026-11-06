package space_world

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
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
	a, collect := newVolumeAdapter(t, maxPending)
	a.so = &localStateSpace{store: kvtx_inmem.NewStore()}
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{}}}
	if err := a.replay.openStage(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.replay.endStage)
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
	f.a.background.Wait()
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
	a.background.Wait()
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
// freed page unless the floor moves past it.
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
}
