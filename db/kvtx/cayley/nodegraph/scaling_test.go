package kvtx_cayley_nodegraph

import (
	"context"
	"runtime"
	"strconv"
	"testing"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// Seeded graph shape of the scaling test: each object links to one of a few
// type hubs and to an earlier parent, so the hubs hold thousands of edges.
const (
	scaleObjects = 2000
	scaleHubs    = 4
	scaleCalls   = 64
	scaleResults = 32
)

// scaleGraph is one seeded graph of the scaling test over a counted store.
type scaleGraph struct {
	h       *cayley.Handle
	store   *kvtx_kvtest.CountingStore
	objects int
}

// scaleCost is the average cost of one call on one graph.
type scaleCost struct {
	reads, steps, writes, allocs float64
}

// scaleObject, scaleHub and the predicates name the seeded values.
func scaleObject(i int) quad.Value { return quad.IRI("object/" + strconv.Itoa(i)) }
func scaleHub(i int) quad.Value    { return quad.IRI("type/" + strconv.Itoa(i%scaleHubs)) }

var (
	scaleTypePred   = quad.IRI("type")
	scaleParentPred = quad.IRI("parent")
)

// newScaleGraph seeds a graph of objects objects in one write transaction of a
// counted in-memory store.
func newScaleGraph(t *testing.T, objects int) *scaleGraph {
	t.Helper()
	ctx := t.Context()
	store := kvtx_kvtest.NewCountingStore(store_kvtx_inmem.NewStore())
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	h, err := NewGraph(tx, nil)
	if err != nil {
		t.Fatal(err)
	}
	quads := make([]quad.Quad, 0, 2*objects)
	for i := range objects {
		quads = append(quads, quad.Make(scaleObject(i), scaleTypePred, scaleHub(i), nil))
		if i != 0 {
			quads = append(quads, quad.Make(scaleObject(i), scaleParentPred, scaleObject(i/2), nil))
		}
	}
	if err := h.AddQuadSet(ctx, quads); err != nil {
		t.Fatal(err)
	}
	return &scaleGraph{h: h, store: store, objects: objects}
}

// pick returns a seed object for call i, spread over the graph and distinct
// for each i below its size.
func (g *scaleGraph) pick(i int) int {
	return 1 + i*7919%(g.objects-1)
}

// lookup runs filter as the World's single quad lookup does and checks that it
// returns want quads.
func (g *scaleGraph) lookup(ctx context.Context, filter quad.Quad, limit uint32, want int) error {
	quads, err := g.h.QuadStore.(*QuadStore).CollectFilteredQuadsBatch(ctx, []quad.Quad{filter}, limit)
	if err == nil && len(quads[0]) != want {
		err = errors.Errorf("lookup returned %d quads, want %d", len(quads[0]), want)
	}
	return err
}

// TestScaling runs each graph entry point of the World's cost table on a graph
// of n objects and one of 10n and checks that a call's key reads, steps and
// writes are the same on both, and its allocations near equal: each follows
// the call's own quads and results, never the graph.
func TestScaling(t *testing.T) {
	ctx := t.Context()
	graphs := [2]*scaleGraph{newScaleGraph(t, scaleObjects), newScaleGraph(t, 10*scaleObjects)}

	// Each entry point, as one call on one graph.
	ops := []struct {
		name string
		op   func(g *scaleGraph, i int) error
	}{
		{"set a quad", func(g *scaleGraph, i int) error {
			return g.h.AddQuad(ctx, quad.Make(scaleObject(g.pick(i)), quad.IRI("link"), scaleObject(g.pick(i+g.objects/2)), nil))
		}},
		{"set a quad naming a new value", func(g *scaleGraph, i int) error {
			return g.h.AddQuad(ctx, quad.Make(scaleObject(g.pick(i)), quad.IRI("tag"), quad.IRI("tag/"+strconv.Itoa(i)), nil))
		}},
		{"delete a quad", func(g *scaleGraph, i int) error {
			k := g.pick(i)
			return g.h.RemoveQuad(ctx, quad.Make(scaleObject(k), scaleParentPred, scaleObject(k/2), nil))
		}},
		{"look up by subject and predicate", func(g *scaleGraph, i int) error {
			return g.lookup(ctx, quad.Make(scaleObject(g.pick(i)), scaleTypePred, nil, nil), 0, 1)
		}},
		{"look up by object and predicate", func(g *scaleGraph, i int) error {
			return g.lookup(ctx, quad.Make(nil, scaleTypePred, scaleHub(i), nil), scaleResults, scaleResults)
		}},
		{"look up one exact quad", func(g *scaleGraph, i int) error {
			k := g.pick(i)
			return g.lookup(ctx, quad.Make(scaleObject(k), scaleTypePred, scaleHub(k), nil), 0, 1)
		}},
	}
	for _, o := range ops {
		var costs [2]scaleCost
		for gi, g := range graphs {
			costs[gi] = measureScale(t, g, o.op)
		}
		small, large := costs[0], costs[1]
		t.Logf("%s: n %+v; 10n %+v", o.name, small, large)
		switch {
		case large.reads > small.reads+0.5, large.steps > small.steps+0.5, large.writes > small.writes+0.5:
			t.Errorf("%s makes %+v key operations a call at 10n against %+v at n", o.name, large, small)
		case large.allocs > small.allocs*1.25+1:
			t.Errorf("%s allocates %.1f objects a call at 10n against %.1f at n", o.name, large.allocs, small.allocs)
		}
	}
}

// measureScale makes scaleCalls calls of op on g and returns their average
// key operations and allocations.
func measureScale(t *testing.T, g *scaleGraph, op func(g *scaleGraph, i int) error) scaleCost {
	t.Helper()
	var before, after runtime.MemStats
	counts := g.store.Counts()
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := range scaleCalls {
		if err := op(g, i); err != nil {
			t.Fatal(err)
		}
	}
	runtime.ReadMemStats(&after)
	d := g.store.Counts().Sub(counts)
	return scaleCost{
		reads:  float64(d.Reads) / scaleCalls,
		steps:  float64(d.Steps) / scaleCalls,
		writes: float64(d.Writes) / scaleCalls,
		allocs: float64(after.Mallocs-before.Mallocs) / scaleCalls,
	}
}
