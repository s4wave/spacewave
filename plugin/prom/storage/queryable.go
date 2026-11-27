package promstorage

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/storage"
	"github.com/s4wave/spacewave/db/block"
	block_kvtx "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
)

// Queryable is a storage.Queryable over the committed segments of several
// targets. It reads the World and keeps no state of its own, so it sees every
// commit the engine has and works against a synced copy of the World.
type Queryable struct {
	// eng is the World engine holding the targets.
	eng world.Engine
	// targets lists the object keys of the targets to read.
	targets TargetLister
}

// NewQueryable constructs a queryable over the targets that targets lists.
func NewQueryable(eng world.Engine, targets TargetLister) *Queryable {
	return &Queryable{eng: eng, targets: targets}
}

// Querier returns a querier for samples between mint and maxt, inclusive, in
// milliseconds.
func (q *Queryable) Querier(mint, maxt int64) (storage.Querier, error) {
	return NewQuerier(q, mint, maxt), nil
}

// forEachTarget calls fn with a reader of each listed target that exists. All
// targets are read in one World read transaction, so a query sees one
// consistent state of the World.
func (q *Queryable) forEachTarget(ctx context.Context, fn func(*StoreReader) error) error {
	// List the targets, then open the single read transaction.
	keys, err := q.targets.ListTargets(ctx)
	if err != nil {
		return err
	}
	wtx, err := q.eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer wtx.Discard()

	// Read each target that exists through its tree.
	for _, key := range keys {
		obj, found, err := wtx.GetObject(ctx, key)
		if err != nil || !found {
			world.ReleaseObjectState(obj)
			if err != nil {
				return errors.Wrapf(err, "target %q", key)
			}
			continue
		}
		_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
			kv, err := block_kvtx.BuildKvTransaction(ctx, bcs, false)
			if err != nil {
				return err
			}
			defer kv.Discard()
			return fn(NewStoreReader(kv))
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return errors.Wrapf(err, "read target %q", key)
		}
	}
	return nil
}

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

// _ checks the implemented interfaces.
var _ TargetLister = StaticTargets(nil)
var _ storage.Queryable = (*Queryable)(nil)
