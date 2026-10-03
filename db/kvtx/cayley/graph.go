package kvtx_cayley

import (
	"context"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/cayley/writer"
	"github.com/s4wave/spacewave/db/kvtx"
	hidalgo "github.com/s4wave/spacewave/db/kvtx/hidalgo"
)

// NewGraph builds a new graph store from a kvtx store.
func NewGraph(
	ctx context.Context,
	objStore kvtx.Store,
	graphOpts graph.Options,
) (*cayley.Handle, error) {
	// Initialize the Cayley database over the transaction store.
	hidalgoKv := flat.Upgrade(hidalgo.NewKV(objStore))
	if err := cayley_kv.Init(ctx, hidalgoKv, graphOpts); err != nil {
		if err != graph.ErrDatabaseExists {
			return nil, err
		}
	}

	// Open the initialized Cayley quad store.
	quadStore, err := cayley_kv.New(ctx, hidalgoKv, graphOpts)
	if err != nil {
		return nil, err
	}

	// Create the quad writer with the graph options.
	// respects ignore_missing ignore_duplicate
	quadWriter, err := writer.NewSingleReplication(quadStore, graphOpts)
	if err != nil {
		return nil, err
	}

	return &cayley.Handle{QuadWriter: quadWriter, QuadStore: quadStore}, nil
}
