package kvtx_cayley

import (
	"context"
	"testing"

	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/graph/iterator"
	"github.com/cayleygraph/cayley/query/path"
	"github.com/cayleygraph/quad"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestCayleyGraph_Basic performs a basic cayley test.
func TestCayleyGraph_Basic(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// build the cayley database
	inMem := store_kvtx_inmem.NewStore()
	objStore := kvtx_vlogger.NewVLogger(le, inMem)
	graphOptions := graph.Options{}
	graph, err := NewGraph(objStore, graphOptions)
	if err != nil {
		t.Fatal(err.Error())
	}

	// graph is the cayley graph.
	// perform the example hello_world from the cayley repository:
	store := graph

	store.AddQuad(quad.Make("phrase of the day", "is of course", "Hello World!", nil))
	store.AddQuad(quad.Make("phrase of the day", "is of course", "I like trains!", nil))

	// Create path querying the data.
	p := cayley.
		StartPath(store, quad.String("phrase of the day")).
		Out(quad.String("is of course"))

	// Now we iterate over results. Arguments:
	// 1. Optional context used for cancellation.
	// 2. Quad store, but we can omit it because we have already built path with it.
	nvals := 0
	err = p.Iterate(nil).EachValue(nil, func(value quad.Value) error {
		nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
		le.Info(nativeValue)
		nvals++
		return nil
	})
	if err == nil && nvals != 2 {
		err = errors.Errorf("expected 2 values but got %d", nvals)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	iterateShape := func(shape iterator.Shape) int {
		itt := shape.Iterate()
		var nm int
		for itt.Next(ctx) {
			nv, err := store.NameOf(itt.Result())
			if err != nil {
				t.Fatal(err.Error())
			}
			t.Logf("value: %v", nv)
			nm++
		}
		if err := itt.Err(); err != nil {
			t.Fatal(err.Error())
		}
		return nm
	}

	// Test a path selecting all nodes in the db.
	shape := store.NodesAllIterator()
	nodesAllN := iterateShape(shape)

	pshape := path.NewPath(store).Shape().BuildIterator(store)
	shapeN := iterateShape(pshape)
	if shapeN != nodesAllN {
		t.Fatalf("got %d nodes", shapeN)
	}

	t.Logf("total matched nodes: %v", nodesAllN)
	if nodesAllN != 4 {
		t.Fatalf("expected 4 nodes but got %d", nodesAllN)
	}
}
