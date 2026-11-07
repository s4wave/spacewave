package world_block

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestEngineStepGraphSettings adds the predicate index to a local World one
// step at a time with writes between the steps, then drops it while another
// goroutine writes, checking lookups against the written quads throughout.
func TestEngineStepGraphSettings(t *testing.T) {
	// Start a testbed with an empty World.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Release)

	// Open an engine on the World.
	eng, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	// Write a starting graph.
	w := &stepTestWriter{t: t, eng: eng, quads: map[string]bool{}}
	for range 20 {
		w.write(ctx)
	}

	// Add the index in small steps, writing and checking between them.
	target, err := NewGraphSettings(predicateIndexes)
	if err != nil {
		t.Fatal(err)
	}
	steps := 0
	for done := false; !done; steps++ {
		done, err = eng.StepGraphSettings(ctx, target, 7)
		if err != nil {
			t.Fatal(err)
		}
		w.write(ctx)
		w.check(ctx)
	}
	if steps < 3 {
		t.Fatalf("build finished in %d steps; want several", steps)
	}
	w.requireSettings(ctx, target, predicateIndexes)

	// A finished change is a no-op that commits nothing.
	head := w.rootRef(ctx)
	if done, err := eng.StepGraphSettings(ctx, target, 7); err != nil || !done {
		t.Fatalf("repeat step: done=%v err=%v", done, err)
	}
	if !w.rootRef(ctx).EqualVT(head) {
		t.Fatal("repeat step changed the World")
	}

	// Write from another goroutine while the index drops.
	dropFrom := w.next
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			w.write(ctx)
		}
	})

	// Drop the index in small steps, checking between them.
	dropSteps := 0
	for done := false; !done; dropSteps++ {
		done, err = eng.StepGraphSettings(ctx, nil, 5)
		if err != nil {
			t.Fatal(err)
		}
		w.check(ctx)
	}
	close(stop)
	wg.Wait()
	t.Logf("build %d steps; drop %d steps with %d concurrent writes", steps, dropSteps, w.next-dropFrom)
	w.check(ctx)
	w.requireSettings(ctx, nil, cayley_kv.DefaultQuadIndexes)
}

// stepTestWriter writes deterministic graph changes to an engine and checks
// its lookups against the quads it wrote.
type stepTestWriter struct {
	t   *testing.T
	eng *Engine
	// mtx guards quads.
	mtx sync.Mutex
	// quads are the live quads, formatted as quadStrings does.
	quads map[string]bool
	// next is the index of the next change; one goroutine writes at a time.
	next int
}

// write applies the next change: it creates an object and links it under
// one of three predicates, deleting a link every fourth change.
func (w *stepTestWriter) write(ctx context.Context) {
	// Open a write transaction.
	i := w.next
	w.next++
	tx, err := w.eng.NewTransaction(ctx, true)
	if err != nil {
		w.t.Error(err)
		return
	}
	defer tx.Discard()

	// Create the object.
	key := fmt.Sprintf("step/obj-%d", i)
	obj, err := tx.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		w.t.Error(err)
		return
	}

	// Link it to earlier objects and remove an older link.
	var set, del []world.GraphQuad
	for j := max(0, i-3); j < i; j++ {
		set = append(set, world.NewGraphQuadWithKeys(key, fmt.Sprintf("<step/rel-%d>", (i+j)%3), fmt.Sprintf("step/obj-%d", j), ""))
	}
	if i%4 == 3 && i >= 4 {
		del = append(del, world.NewGraphQuadWithKeys(fmt.Sprintf("step/obj-%d", i-2), fmt.Sprintf("<step/rel-%d>", (2*i-5)%3), fmt.Sprintf("step/obj-%d", i-3), ""))
	}
	for _, q := range set {
		if err := tx.SetGraphQuad(ctx, q); err != nil {
			w.t.Error(err)
			return
		}
	}
	for _, q := range del {
		if err := tx.DeleteGraphQuad(ctx, q); err != nil {
			w.t.Error(err)
			return
		}
	}

	// Commit and record the result.
	if err := tx.Commit(ctx); err != nil {
		w.t.Error(err)
		return
	}
	w.mtx.Lock()
	defer w.mtx.Unlock()
	for _, s := range quadStrings(set) {
		w.quads[s] = true
	}
	for _, s := range quadStrings(del) {
		delete(w.quads, s)
	}
}

// check compares each predicate's lookup with the written quads. A snapshot
// of the written quads is taken under the same read transaction as the
// lookups so a concurrent write cannot fall between them.
func (w *stepTestWriter) check(ctx context.Context) {
	// Open a read transaction and snapshot the written quads.
	w.t.Helper()
	w.mtx.Lock()
	tx, err := w.eng.NewTransaction(ctx, false)
	var want []string
	for s := range w.quads {
		want = append(want, s)
	}
	w.mtx.Unlock()
	if err != nil {
		w.t.Fatal(err)
	}
	defer tx.Discard()

	// Look up every predicate and compare the union.
	var got []string
	for p := range 3 {
		quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuad("", fmt.Sprintf("<step/rel-%d>", p), "", ""), 0)
		if err != nil {
			w.t.Fatal(err)
		}
		got = append(got, quadStrings(quads)...)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		w.t.Fatalf("lookups returned %d quads, want %d:\n got %v\nwant %v", len(got), len(want), got, want)
	}
}

// requireSettings checks the root's settings and the layout a new
// transaction opens the graph with.
func (w *stepTestWriter) requireSettings(ctx context.Context, settings *GraphSettings, layout []cayley_kv.QuadIndex) {
	// Open a read transaction on the current World.
	w.t.Helper()
	etx, err := w.eng.NewBlockEngineTransaction(ctx, false)
	if err != nil {
		w.t.Fatal(err)
	}
	defer etx.Discard()
	ws := etx.readTx.Load().state

	// Compare the root and the open graph.
	root, err := ws.GetRoot(ctx)
	if err != nil {
		w.t.Fatal(err)
	}
	if !root.GetGraphSettings().EqualVT(settings) {
		w.t.Fatalf("root graph settings %v, want %v", root.GetGraphSettings(), settings)
	}
	graphHd, err := ws.graph(ctx)
	if err != nil {
		w.t.Fatal(err)
	}
	if got := graphHd.QuadStore.(*cayley_kv.QuadStore).Layout(); !cayley_kv.CompareQuadIndexes(got, layout) {
		w.t.Fatalf("open graph layout %v", got)
	}
	if got := readStoredLayout(w.t, ctx, ws); !cayley_kv.CompareQuadIndexes(got, layout) {
		w.t.Fatalf("stored graph layout %v", got)
	}
}

// rootRef returns the engine's current root reference.
func (w *stepTestWriter) rootRef(ctx context.Context) *block.BlockRef {
	// Read the root through a read transaction.
	w.t.Helper()
	etx, err := w.eng.NewBlockEngineTransaction(ctx, false)
	if err != nil {
		w.t.Fatal(err)
	}
	defer etx.Discard()
	return etx.readTx.Load().state.GetRootRef().Clone()
}
