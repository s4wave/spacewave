package prom_plugin

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
)

// TargetLister lists the World object keys of the targets a Queryable reads.
// The Space's set of prom/target objects is the list. A lister is where that
// set comes from.
type TargetLister interface {
	// ListTargets returns the object keys of the targets to query. A key whose
	// object does not exist is skipped by the query.
	ListTargets(ctx context.Context) ([]string, error)
}

// StaticTargets is a TargetLister with a fixed list of object keys.
type StaticTargets []string

// ListTargets returns the fixed object keys.
func (t StaticTargets) ListTargets(context.Context) ([]string, error) {
	return t, nil
}

// _ is a type assertion
var _ TargetLister = StaticTargets(nil)

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
var _ TargetLister = worldTargets{}
