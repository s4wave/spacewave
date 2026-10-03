package kvtx_cayley

import (
	"context"
	"testing"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/iterator"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/path"
	"github.com/pkg/errors"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	"github.com/sirupsen/logrus"
)

// TestCayleyGraph_Basic performs a basic cayley test.
func TestCayleyGraph_Basic(t *testing.T) {
	// Prepare the graph test context and debug logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// build the cayley database
	inMem := store_kvtx_inmem.NewStore()
	objStore := kvtx_vlogger.NewVLogger(le, inMem)
	graphOptions := graph.Options{}
	graph, err := NewGraph(ctx, objStore, graphOptions)
	if err != nil {
		t.Fatal(err.Error())
	}

	// graph is the cayley graph.
	// perform the example hello_world from the cayley repository:
	store := graph

	// Populate the graph with two phrase values.
	_ = store.AddQuad(ctx, quad.Make("phrase of the day", "is of course", "Hello World!", nil))
	_ = store.AddQuad(ctx, quad.Make("phrase of the day", "is of course", "I like trains!", nil))

	// Create path querying the data.
	p := cayley.
		StartPath(store, quad.String("phrase of the day")).
		Out(quad.String("is of course"))

	// Now we iterate over results. Arguments:
	// 1. Optional context used for cancellation.
	// 2. Quad store, but we can omit it because we have already built path with it.
	nvals := 0
	err = p.Iterate(ctx).EachValue(ctx, nil, func(value quad.Value) error {
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

	// Count the nodes yielded by a graph iterator.
	iterateShape := func(shape iterator.Shape) int {
		// Open the shape iterator and count its resolved nodes.
		itt := shape.Iterate(ctx)
		var nm int
		for itt.Next(ctx) {
			// Read the next graph node from the iterator.
			resi, err := itt.Result(ctx)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Resolve the graph node to its named value.
			nv, err := store.NameOf(ctx, resi)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Record the resolved node in the iterator count.
			t.Logf("value: %v", nv)
			nm++
		}

		// Require the graph iterator to finish without an error.
		if err := itt.Err(); err != nil {
			t.Fatal(err.Error())
		}

		return nm
	}

	// Test a path selecting all nodes in the db.
	shape := store.NodesAllIterator(ctx)
	nodesAllN := iterateShape(shape)

	// Compare the empty path with the all-nodes iterator.
	pshape := path.NewPath(store).Shape().BuildIterator(ctx, store)
	shapeN := iterateShape(pshape)
	if shapeN != nodesAllN {
		t.Fatalf("got %d nodes", shapeN)
	}

	// Verify the graph contains the four distinct fixture nodes.
	t.Logf("total matched nodes: %v", nodesAllN)
	if nodesAllN != 4 {
		t.Fatalf("expected 4 nodes but got %d", nodesAllN)
	}
}
