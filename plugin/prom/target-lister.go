package prom_plugin

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	promstorage "github.com/s4wave/spacewave/plugin/prom/storage"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
)

// worldTargets lists every prom/target object in a World.
type worldTargets struct {
	// eng is the World engine to list.
	eng world.Engine
}

// ListTargets returns the keys of the World's prom/target objects.
func (t worldTargets) ListTargets(ctx context.Context) ([]string, error) {
	tx, err := t.eng.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	return s4wave_prom.ListTargets(ctx, tx)
}

// _ is a type assertion
var _ promstorage.TargetLister = worldTargets{}
