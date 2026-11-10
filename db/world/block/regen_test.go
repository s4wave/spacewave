//go:build !js && !wasip1

package world_block

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

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

// TestRegeneratorSwapsWhileCommitting regenerates a kv World on a Volume that
// frees at once while a writer commits between every step, through a
// restart at each stage, and checks that the head becomes a node graph World
// holding exactly the objects and quads committed, that the engine follows it
// and keeps writing, and that collection between steps loses nothing.
func TestRegeneratorSwapsWhileCommitting(t *testing.T) {
	// Seed a kv World with objects and quads, some labeled.
	ctx := t.Context()
	f := newVolumeFixture(t)
	authority := f.authority()
	engine := f.openEngine(t, authority)
	m := &regenModel{objects: make(map[string]bool), quads: make(map[string]world.GraphQuad)}
	keys := fixtureKeys("o", 20)
	m.commit(t, engine, func(w *EngineTx, add, _ func(world.GraphQuad)) {
		m.create(t, w, keys...)
		for i := range 40 {
			label := ""
			if i%5 == 0 {
				label = fmt.Sprintf("<l%d>", i%2)
			}
			add(regenQuad(keys[i%20], fmt.Sprintf("<p%d>", i%3), keys[(i*7+1)%20], label))
		}
	})
	if f := headFormat(t, engine); f != WorldFormat_WORLD_FORMAT_CAYLEY_KV {
		t.Fatalf("seeded World is %v", f)
	}
	cfg := RegenConfig{SpillDir: t.TempDir(), Budget: 1 << 20}
	stage := func(r *Regenerator, want RegenStage) regenState {
		t.Helper()
		s, err := r.read(ctx)
		if err != nil || s.stage != want {
			t.Fatalf("stage %v, %v; want %v", s.stage, err, want)
		}
		return s
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
	f.collect(t)
	m.commit(t, engine, func(w *EngineTx, add, del func(world.GraphQuad)) {
		m.create(t, w, "late-1")
		del(regenQuad(keys[5], "<p2>", keys[16], ""))
		add(regenQuad("late-1", "<p0>", "late-0", "<l1>"))
	})

	// Restart, and run a round that leaves too much for the swap.
	r = NewRegenerator(authority, cfg)
	if err := r.catchUp(ctx, stage(r, RegenCatchingUp)); err != nil {
		t.Fatal(err)
	}
	s := stage(r, RegenCatchingUp)
	if head := f.worldRoot(t); !s.base.EqualsRef(head) {
		t.Fatalf("round left base %v behind head %v", s.base, head)
	}
	f.collect(t)
	m.commit(t, engine, func(_ *EngineTx, add, del func(world.GraphQuad)) {
		del(regenQuad(keys[6], "<p0>", keys[3], ""))
		add(regenQuad(keys[19], "<p1>", "late-1", ""))
	})

	// Restart, and run to the swap.
	cfg.SwapChanges = 1 << 10
	r = NewRegenerator(authority, cfg)
	if err := r.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s = stage(r, RegenSwitched)
	if !s.built.EqualsRef(f.worldRoot(t)) || s.base.EqualsRef(s.built) {
		t.Fatalf("swap left built %v base %v head %v", s.built, s.base, f.worldRoot(t))
	}
	f.collect(t)

	// The engine follows the new head and holds exactly what was committed.
	deadline := time.Now().Add(5 * time.Second)
	for !engine.GetRootRef().GetRootRef().EqualsRef(s.built) {
		if time.Now().After(deadline) {
			t.Fatal("engine did not follow the swapped head")
		}
		time.Sleep(time.Millisecond)
	}
	if f := headFormat(t, engine); f != WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		t.Fatalf("swapped World is %v", f)
	}
	m.check(t, engine)

	// It keeps writing on the new World.
	m.commit(t, engine, func(_ *EngineTx, add, del func(world.GraphQuad)) {
		del(readded)
		add(regenQuad("late-0", "<p0>", "late-1", ""))
	})
	f.collect(t)
	m.check(t, engine)
}
