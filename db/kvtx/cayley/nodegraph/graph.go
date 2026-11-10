package kvtx_cayley_nodegraph

import (
	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/writer"
	"github.com/s4wave/spacewave/db/kvtx"
)

// NewGraph opens a graph handle over the store in tx, as NewQuadStore does,
// with a writer that applies each write as one delta batch.
func NewGraph(tx kvtx.TxOps) (*cayley.Handle, error) {
	qs := NewQuadStore(tx)
	w, err := writer.NewSingleReplication(qs, nil)
	if err != nil {
		return nil, err
	}
	return &cayley.Handle{QuadStore: qs, QuadWriter: w}, nil
}
