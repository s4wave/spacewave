package kvtx_cayley

import (
	"testing"

	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/quad"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestCayleyGraph_Basic performs a basic cayley test.
func TestCayleyGraph_Basic(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// build the cayley database
	inMem := store_kvtx_inmem.NewStore()
	objStore := kvtx_vlogger.NewVLogger(le, inMem)
	graphOptions := graph.Options{}
	graph, err := NewGraph(objStore, graphOptions)
	if err != nil {
		panic(err)
	}

	// graph is the cayley graph.
	// perform the example hello_world from the cayley repository:
	store := graph

	store.AddQuad(quad.Make("phrase of the day", "is of course", "Hello World!", nil))
	store.AddQuad(quad.Make("phrase of the day", "is of course", "I like trains!", nil))

	// Now we create the path, to get to our data
	p := cayley.StartPath(store, quad.String("phrase of the day")).Out(quad.String("is of course"))

	// Now we iterate over results. Arguments:
	// 1. Optional context used for cancellation.
	// 2. Quad store, but we can omit it because we have already built path with it.
	nvals := 0
	err = p.Iterate(nil).EachValue(nil, func(value quad.Value) {
		nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
		le.Info(nativeValue)
		nvals++
	})
	if err == nil && nvals != 2 {
		err = errors.Errorf("expected 2 values but got %d", nvals)
	}
	if err != nil {
		panic(err)
	}
}
