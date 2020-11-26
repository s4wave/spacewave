package kvtx_cayley

import (
	"github.com/aperturerobotics/hydra/kvtx"
	hidalgo "github.com/aperturerobotics/hydra/kvtx/hidalgo"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	cayley_kv "github.com/cayleygraph/cayley/graph/kv"
	"github.com/cayleygraph/cayley/writer"
)

// NewGraph builds a new graph store from a kvtx store.
func NewGraph(
	objStore kvtx.Store,
	graphOpts graph.Options,
) (*cayley.Handle, error) {
	hidalgoKv := hidalgo.NewKV(objStore)
	if err := cayley_kv.Init(hidalgoKv, graphOpts); err != nil {
		if err != graph.ErrDatabaseExists {
			return nil, err
		}
	}
	quadStore, err := cayley_kv.New(hidalgoKv, graphOpts)
	if err != nil {
		return nil, err
	}
	// respects ignore_missing ignore_duplicate
	quadWriter, err := writer.NewSingleReplication(quadStore, graphOpts)
	if err != nil {
		return nil, err
	}
	return &cayley.Handle{QuadWriter: quadWriter, QuadStore: quadStore}, nil
}
