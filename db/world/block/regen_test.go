//go:build !js && !wasip1

package world_block

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/s4wave/spacewave/db/block"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
)

// regenModel is the objects and quads a test expects the World to hold.
type regenModel struct {
	// objects are the object keys.
	objects map[string]bool
	// quads are the quads by their text.
	quads map[string]world.GraphQuad
}

// regenQuad returns the quad between objects s and o with predicate p.
func regenQuad(s, p, o, l string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(s, p, o, l)
}

// regenQuadText identifies a quad for comparison.
func regenQuadText(q world.GraphQuad) string {
	return fmt.Sprintf("%s %s %s %s", q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel())
}

// commit runs fn in one write transaction on engine and records its changes
// in the model through the returned adders.
func (m *regenModel) commit(t *testing.T, engine *Engine, fn func(w *EngineTx, add, del func(world.GraphQuad))) {
	// Apply fn's quad changes to the World and the model.
	t.Helper()
	ctx := t.Context()
	w := fixtureWriter(t, engine)
	fn(w, func(q world.GraphQuad) {
		if err := w.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
		m.quads[regenQuadText(q)] = q
	}, func(q world.GraphQuad) {
		if err := w.DeleteGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
		delete(m.quads, regenQuadText(q))
	})
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// create adds object keys in w and the model.
func (m *regenModel) create(t *testing.T, w *EngineTx, keys ...string) {
	t.Helper()
	for _, key := range keys {
		fixtureObject(t, w, key, key)
		m.objects[key] = true
	}
}

// check fails unless a fresh read of engine holds exactly the model.
func (m *regenModel) check(t *testing.T, engine *Engine) {
	// Read every quad and object key.
	t.Helper()
	ctx := t.Context()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	found, err := tx.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
	if err != nil {
		t.Fatal(err)
	}
	var quads []string
	for _, q := range found {
		quads = append(quads, regenQuadText(q))
	}
	var objects []string
	it := tx.IterateObjects(ctx, "", false)
	for it.Next() {
		objects = append(objects, it.Key())
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	it.Close()

	// Compare them with the model.
	slices.Sort(quads)
	if want := slices.Sorted(maps.Keys(m.quads)); !slices.Equal(quads, want) {
		t.Fatalf("quads differ:\n got %v\nwant %v", quads, want)
	}
	if want := slices.Sorted(maps.Keys(m.objects)); !slices.Equal(objects, want) {
		t.Fatalf("objects differ:\n got %v\nwant %v", objects, want)
	}
}

// headFormat returns the format of the World the engine reads.
func headFormat(t *testing.T, engine *Engine) WorldFormat {
	t.Helper()
	tx, err := engine.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	root, err := tx.(*EngineTx).readTx.Load().state.GetRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return root.GetFormat()
}

// regenFixture is a kv World under a writing engine, seeded for regeneration.
type regenFixture struct {
	*volumeFixture
	authority *LocalAuthority
	engine    *Engine
	m         *regenModel
	keys      []string
	cfg       RegenConfig
}

// newRegenFixture seeds a kv World with objects and quads, some labeled.
func newRegenFixture(t *testing.T) *regenFixture {
	f := newVolumeFixture(t)
	authority := f.authority()
	rf := &regenFixture{
		volumeFixture: f,
		authority:     authority,
		engine:        f.openEngine(t, authority),
		m:             &regenModel{objects: make(map[string]bool), quads: make(map[string]world.GraphQuad)},
		keys:          fixtureKeys("o", 20),
		cfg:           RegenConfig{SpillDir: t.TempDir(), Budget: 1 << 20},
	}
	rf.m.commit(t, rf.engine, func(w *EngineTx, add, _ func(world.GraphQuad)) {
		rf.m.create(t, w, rf.keys...)
		for i := range 40 {
			label := ""
			if i%5 == 0 {
				label = fmt.Sprintf("<l%d>", i%2)
			}
			add(regenQuad(rf.keys[i%20], fmt.Sprintf("<p%d>", i%3), rf.keys[(i*7+1)%20], label))
		}
	})
	if f := headFormat(t, rf.engine); f != WorldFormat_WORLD_FORMAT_CAYLEY_KV {
		t.Fatalf("seeded World is %v", f)
	}
	return rf
}

// stage fails unless r reads stage want, and returns the state.
func (rf *regenFixture) stage(t *testing.T, r *Regenerator, want RegenStage) regenState {
	t.Helper()
	s, err := r.read(t.Context())
	if err != nil || s.stage != want {
		t.Fatalf("stage %v, %v; want %v", s.stage, err, want)
	}
	return s
}

// follow waits for the engine to read the World root ref in format.
func (rf *regenFixture) follow(t *testing.T, ref *block.BlockRef, format WorldFormat) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !rf.engine.GetRootRef().GetRootRef().EqualsRef(ref) {
		if time.Now().After(deadline) {
			t.Fatal("engine did not follow the swapped head")
		}
		time.Sleep(time.Millisecond)
	}
	if f := headFormat(t, rf.engine); f != format {
		t.Fatalf("head World is %v, want %v", f, format)
	}
}

// switchTo pins, builds and swaps with no commits between, running fault on
// the built root before the swap when it is set, and returns the switched
// regenerator.
func (rf *regenFixture) switchTo(t *testing.T, fault func(r *Regenerator, s regenState)) *Regenerator {
	t.Helper()
	ctx := t.Context()
	rf.cfg.SwapChanges = 1 << 10
	r := NewRegenerator(rf.authority, rf.cfg)
	if err := r.pin(ctx, rf.stage(t, r, RegenIdle)); err != nil {
		t.Fatal(err)
	}
	if err := r.build(ctx, rf.stage(t, r, RegenBuilding)); err != nil {
		t.Fatal(err)
	}
	if fault != nil {
		fault(r, rf.stage(t, r, RegenCatchingUp))
	}
	rf.m.commit(t, rf.engine, func(_ *EngineTx, add, _ func(world.GraphQuad)) {
		add(regenQuad(rf.keys[2], "<p7>", rf.keys[3], ""))
	})
	if err := r.catchUp(ctx, rf.stage(t, r, RegenCatchingUp), regenForward); err != nil {
		t.Fatal(err)
	}
	s := rf.stage(t, r, RegenSwitched)
	rf.follow(t, s.built, WorldFormat_WORLD_FORMAT_NODE_GRAPH)
	return r
}

// reload opens an engine afresh on the head, as a restart does, which loads
// the head World through checkTreeImpls, and checks that it reads format and
// holds exactly what was committed.
func (rf *regenFixture) reload(t *testing.T, format WorldFormat) {
	t.Helper()
	reopened := rf.openEngine(t, rf.volumeFixture.authority())
	if f := headFormat(t, reopened); f != format {
		t.Fatalf("reloaded head is %v, want %v", f, format)
	}
	rf.m.check(t, reopened)
}

// checkBuilt checks the state a crash between build and graft leaves: an
// engine opened afresh loads the head as the kv World it was, its object tree
// untagged, while the built root carries the node graph's tags. It then
// grafts the head onto the built root, which must write the World block alone,
// since retagging the object tree changes only its root inside that block.
func (rf *regenFixture) checkBuilt(t *testing.T, r *Regenerator, s regenState) {
	t.Helper()
	ctx := t.Context()
	rf.reload(t, WorldFormat_WORLD_FORMAT_CAYLEY_KV)
	head := rf.worldRoot(t)
	_, hw, err := r.readWorld(ctx, r.a.store, head)
	if err != nil {
		t.Fatal(err)
	}
	if err := hw.checkTreeImpls(); err != nil {
		t.Fatalf("kv head: %v", err)
	}

	// A kv World whose object tree carries either layout's node graph tag is
	// refused at load.
	for _, impl := range []kvtx_block.KVImplType{
		kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL_NODE_GRAPH,
		kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_NODE_GRAPH,
	} {
		tagged := hw.CloneVT()
		tagged.ObjectKeyValue.ImplType = impl
		if tagged.checkTreeImpls() == nil {
			t.Fatalf("kv World with a %v object tree loads", impl)
		}
	}
	_, bw, err := r.readWorld(ctx, r.a.store, s.built)
	if err == nil {
		err = bw.checkTreeImpls()
	}
	if err != nil || bw.GetFormat() != WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		t.Fatalf("built World is %v: %v", bw.GetFormat(), err)
	}

	// Graft through a store that counts the blocks written.
	var puts int
	counted := newReadThroughStore(r.a.store, ReadThrough{
		Get: func(ctx context.Context, store block.StoreOps, ref *block.BlockRef) ([]byte, bool, error) {
			return store.GetBlock(ctx, ref)
		},
		Put: func(*block.BlockRef) { puts++ },
	})
	g, err := r.graft(ctx, counted, head, s.built)
	if err != nil {
		t.Fatal(err)
	}
	if puts != 1 {
		t.Fatalf("graft wrote %d blocks, want the World block alone", puts)
	}
	_, gw, err := r.readWorld(ctx, r.a.store, g.ref)
	if err == nil {
		err = gw.checkTreeImpls()
	}
	if err != nil {
		t.Fatalf("grafted World: %v", err)
	}
}

// TestRegeneratorSwapsWhileCommitting regenerates a kv World on a Volume that
// frees at once while a writer commits between every step, through a
// restart at each stage, and checks that the head becomes a node graph World
// holding exactly the objects and quads committed, that the engine follows it
// and keeps writing, that Run verifies and releases the old root, and that
// collection between steps loses nothing.
func TestRegeneratorSwapsWhileCommitting(t *testing.T) {
	ctx := t.Context()
	rf := newRegenFixture(t)
	f, engine, m, keys, cfg := rf.volumeFixture, rf.engine, rf.m, rf.keys, rf.cfg
	authority, stage := rf.authority, func(r *Regenerator, want RegenStage) regenState {
		t.Helper()
		return rf.stage(t, r, want)
	}

	// Pin, then commit deletes, adds and a quad deleted and added again.
	r := NewRegenerator(authority, cfg)
	if err := r.pin(ctx, stage(r, RegenIdle)); err != nil {
		t.Fatal(err)
	}
	readded := regenQuad(keys[0], "<p0>", keys[1], "<l0>")
	m.commit(t, engine, func(w *EngineTx, add, del func(world.GraphQuad)) {
		m.create(t, w, "late-0")
		del(readded)
		del(regenQuad(keys[3], "<p0>", keys[2], ""))
		add(regenQuad("late-0", "<p9>", keys[4], ""))
	})
	m.commit(t, engine, func(_ *EngineTx, add, _ func(world.GraphQuad)) {
		add(readded)
	})
	f.collect(t)

	// Restart, and build the pin while the writer commits.
	r = NewRegenerator(authority, cfg)
	if err := r.build(ctx, stage(r, RegenBuilding)); err != nil {
		t.Fatal(err)
	}
	rf.checkBuilt(t, r, stage(r, RegenCatchingUp))
	f.collect(t)
	m.commit(t, engine, func(w *EngineTx, add, del func(world.GraphQuad)) {
		m.create(t, w, "late-1")
		del(regenQuad(keys[5], "<p2>", keys[16], ""))
		add(regenQuad("late-1", "<p0>", "late-0", "<l1>"))
	})

	// Restart, and run a round that leaves too much for the swap.
	r = NewRegenerator(authority, cfg)
	if err := r.catchUp(ctx, stage(r, RegenCatchingUp), regenForward); err != nil {
		t.Fatal(err)
	}
	s := stage(r, RegenCatchingUp)
	if head := f.worldRoot(t); !s.base.EqualsRef(head) {
		t.Fatalf("round left base %v behind head %v", s.base, head)
	}
	// A crash before the switch reloads the head as the old kv World.
	rf.reload(t, WorldFormat_WORLD_FORMAT_CAYLEY_KV)
	f.collect(t)
	m.commit(t, engine, func(_ *EngineTx, add, del func(world.GraphQuad)) {
		del(regenQuad(keys[6], "<p0>", keys[3], ""))
		add(regenQuad(keys[19], "<p1>", "late-1", ""))
	})

	// Restart, and run the round that swaps.
	cfg.SwapChanges = 1 << 10
	r = NewRegenerator(authority, cfg)
	if err := r.catchUp(ctx, stage(r, RegenCatchingUp), regenForward); err != nil {
		t.Fatal(err)
	}
	s = stage(r, RegenSwitched)
	if !s.built.EqualsRef(f.worldRoot(t)) || s.base.EqualsRef(s.built) {
		t.Fatalf("swap left built %v base %v head %v", s.built, s.base, f.worldRoot(t))
	}
	f.collect(t)

	// The engine follows the new head and holds exactly what was committed.
	rf.follow(t, s.built, WorldFormat_WORLD_FORMAT_NODE_GRAPH)
	m.check(t, engine)

	// It keeps writing on the new World.
	m.commit(t, engine, func(_ *EngineTx, add, del func(world.GraphQuad)) {
		del(readded)
		add(regenQuad("late-0", "<p0>", "late-1", ""))
	})
	f.collect(t)
	m.check(t, engine)

	// Run proves the new World equal to the old root and releases it.
	if err := NewRegenerator(authority, cfg).Run(ctx, RegenDone, nil); err != nil {
		t.Fatal(err)
	}
	stage(r, RegenDone)
	f.collect(t)
	m.check(t, engine)

	// A restart loads the finished World through checkTreeImpls.
	rf.reload(t, WorldFormat_WORLD_FORMAT_NODE_GRAPH)
}

// TestRegeneratorSwitchesBack commits on a switched World, including
// deleting the nodes with the highest IDs and adding new ones, runs a switch
// back round that leaves too much for the turn, commits again, and switches
// back. The head must be a kv World holding exactly what was committed, keep
// writing, and release to idle.
func TestRegeneratorSwitchesBack(t *testing.T) {
	// Switch, then commit on the new World: drop the only quad naming the
	// newest node, <p7>, and add quads on new ones.
	ctx := t.Context()
	rf := newRegenFixture(t)
	rf.switchTo(t, nil)
	rf.m.commit(t, rf.engine, func(w *EngineTx, add, del func(world.GraphQuad)) {
		del(regenQuad(rf.keys[2], "<p7>", rf.keys[3], ""))
		rf.m.create(t, w, "back-0")
		add(regenQuad("back-0", "<p8>", rf.keys[1], "<l2>"))
	})

	// A round off the turn leaves too much for a zero bound and moves built
	// to the head.
	rf.cfg.SwapChanges = 0
	r := NewRegenerator(rf.authority, rf.cfg)
	if err := r.catchUp(ctx, rf.stage(t, r, RegenSwitched), regenBack); err != nil {
		t.Fatal(err)
	}
	s := rf.stage(t, r, RegenSwitched)
	if !s.built.EqualsRef(rf.worldRoot(t)) {
		t.Fatalf("back round left built %v behind head %v", s.built, rf.worldRoot(t))
	}
	rf.collect(t)
	rf.m.commit(t, rf.engine, func(_ *EngineTx, add, del func(world.GraphQuad)) {
		del(regenQuad("back-0", "<p8>", rf.keys[1], "<l2>"))
		add(regenQuad(rf.keys[9], "<p9>", "back-0", ""))
	})

	// Switch back under the turn; the old format serves what was committed.
	rf.cfg.SwapChanges = 1 << 10
	r = NewRegenerator(rf.authority, rf.cfg)
	if err := r.SwitchBack(ctx); err != nil {
		t.Fatal(err)
	}
	s = rf.stage(t, r, RegenSwitchedBack)
	if !s.base.GetEmpty() || s.built.EqualsRef(rf.worldRoot(t)) {
		t.Fatalf("switch back left base %v built %v head %v", s.base, s.built, rf.worldRoot(t))
	}
	rf.follow(t, rf.worldRoot(t), WorldFormat_WORLD_FORMAT_CAYLEY_KV)
	rf.collect(t)
	rf.m.check(t, rf.engine)
	if err := r.Run(ctx, RegenDone, nil); !errors.Is(err, ErrRegenSwitchedBack) {
		t.Fatalf("Run after switch back: %v", err)
	}

	// It keeps writing, and Release lets go of the new-format root.
	rf.m.commit(t, rf.engine, func(_ *EngineTx, add, _ func(world.GraphQuad)) {
		add(regenQuad("back-0", "<p0>", rf.keys[4], ""))
	})
	if err := r.Release(ctx); err != nil {
		t.Fatal(err)
	}
	rf.stage(t, r, RegenIdle)
	rf.collect(t)
	rf.m.check(t, rf.engine)
}

// TestRegeneratorMismatchSwitchesBack swaps in a built World missing a quad
// and checks that Run finds the difference, switches back to the old root,
// and returns ErrRegenMismatch, with the head holding every quad.
func TestRegeneratorMismatchSwitchesBack(t *testing.T) {
	// Drop one quad from the built World before the swap.
	ctx := t.Context()
	rf := newRegenFixture(t)
	dropped := rf.m.quads[slices.Sorted(maps.Keys(rf.m.quads))[0]]
	r := rf.switchTo(t, func(r *Regenerator, s regenState) {
		store, release, err := block.OpenStage(ctx, r.a.store)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		g, err := r.graft(ctx, store, s.built, s.built)
		if err != nil {
			t.Fatal(err)
		}
		g.limit = -1
		q, err := world.GraphQuadToCayleyQuad(dropped, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := g.apply(ctx, graph.Delta{Quad: q, Action: graph.Delete}); err != nil {
			t.Fatal(err)
		}
		if _, err := g.write(ctx); err != nil {
			t.Fatal(err)
		}
		if err := r.a.store.SetRoots(ctx, nil, []block.NamedRoot{{Name: r.builtName, Ref: g.ref, Previous: s.built}}); err != nil {
			t.Fatal(err)
		}
	})

	// Run finds the missing quad and switches back.
	if err := r.Run(ctx, RegenDone, nil); !errors.Is(err, ErrRegenMismatch) {
		t.Fatalf("Run on a faulty World: %v", err)
	}
	rf.stage(t, r, RegenSwitchedBack)
	rf.follow(t, rf.worldRoot(t), WorldFormat_WORLD_FORMAT_CAYLEY_KV)
	rf.m.check(t, rf.engine)
}
