package kvtx_genji

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/genjidb/genji"
)

// NewGenjiDB builds a new genji database from a kvtx store.
func NewGenjiDB(ctx context.Context, kvtx kvtx.Store) (*genji.DB, error) {
	return genji.New(ctx, NewEngine(kvtx))
}
