//go:build !js

package device_flowgraph

import (
	"context"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// claimRaceTx rejects the first candidate before the real World commit.
type claimRaceTx struct {
	// Tx retains the actual testbed transaction and stale-root checks.
	world.Tx
	// engine coordinates the first two competing placement candidates.
	engine *claimRaceEngine
}

// Commit simulates a stale root on the first competing placement.
func (tx *claimRaceTx) Commit(ctx context.Context) error {
	// Ignore other edits, including Device capability projection and earlier claims.
	snapshot, err := s4wave_flowgraph.ReadFlowgraph(ctx, tx.Tx, testFlowgraphKey)
	if err != nil {
		return err
	}
	if snapshot.GetPlacements()["race"] == nil {
		return tx.Tx.Commit(ctx)
	}

	// Refuse the first candidate so World retries the entire placement and debit.
	if tx.engine.candidates.Add(1) == 1 {
		return coord.ErrStaleGeneration
	}

	return tx.Tx.Commit(ctx)
}
