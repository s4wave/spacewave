//go:build !js

package device_flowgraph

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/world"
)

// claimRaceEngine injects a superseded World root during the competing claim.
// The testbed serializes write transactions, so a barrier inside Commit would
// block the second claimant before it can open its transaction.
type claimRaceEngine struct {
	// Engine supplies the real transactional World.
	world.Engine
	// candidates counts commits that contain the competing node's placement.
	candidates atomic.Int32
}

// NewTransaction wraps writes while preserving the testbed's commit semantics.
func (e *claimRaceEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	// Wrap only write transactions; watches still read the real World directly.
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &claimRaceTx{Tx: tx, engine: e}, nil
}
