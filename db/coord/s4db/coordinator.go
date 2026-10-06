// Package s4db coordinates the writers of a Volume stored in one s4db file
// through the file itself.
//
// A write lease holds a lease of the database, so one process at a time holds
// each scope and the system releases it when the holder dies. The generation
// of an ObjectStore scope is the database's commit sequence, which every
// process sharing the file observes. Root and key prefix events of the
// processes sharing the coordinator's in-memory inner coordinator ride along
// with it; other processes learn of a change from the commit that wrote it.
package s4db

import (
	"context"

	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	db_s4db "github.com/s4wave/spacewave/db/s4db"
)

// Coordinator coordinates the writers of one s4db database file.
type Coordinator struct {
	// db is the database file.
	db *db_s4db.DB
	// inner holds this process's roots, events, and lease turns.
	inner *coord_inmem.Coordinator
}

// NewCoordinator builds a coordinator over db. Coordinators of one Volume in
// a process share inner, which carries their roots and events.
func NewCoordinator(db *db_s4db.DB, inner *coord_inmem.Coordinator) *Coordinator {
	return &Coordinator{db: db, inner: inner}
}

// Capability reports support for every scope and generations for
// ObjectStore scopes.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &coord.Capability{
		Supported:     true,
		Backend:       coord.BackendKindS4db,
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		Generation:    c.db.Seq(),
		Generations:   scope.Key == "",
	}, nil
}

// Snapshot returns the inner root at the database's commit sequence.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	snapshot, err := c.inner.Snapshot(ctx, scope)
	if err != nil {
		return nil, err
	}
	if scope.Key == "" {
		snapshot.Generation = c.db.Seq()
	}
	return snapshot, nil
}

// Watch streams the inner events and every commit to the database after
// afterGeneration, each carrying the commit sequence. Keyed scopes have no
// events.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	inner, err := c.inner.Watch(ctx, scope, afterGeneration)
	if err != nil || scope.Key != "" {
		return inner, err
	}
	return newWatch(ctx, c.db, scope, inner, afterGeneration), nil
}

// TryAcquireWriteLease takes the inner lease and the database lease of scope
// when neither is held.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	// Take this process's turn.
	inner, ok, err := c.inner.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		return nil, ok, err
	}

	// Take the scope in the file.
	held, ok, err := c.db.TryLease(leaseName(scope))
	if err != nil || !ok {
		_ = inner.Release(context.Background())
		return nil, false, err
	}
	return &lease{db: c.db, scope: scope, inner: inner, held: held}, true, nil
}

// WaitAcquireWriteLease waits for this process's turn, then for other
// processes to release scope.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	// Wait for this process's turn.
	inner, err := c.inner.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Wait for the scope in the file.
	held, err := c.db.WaitLease(ctx, leaseName(scope))
	if err != nil {
		_ = inner.Release(context.Background())
		return nil, err
	}
	return &lease{db: c.db, scope: scope, inner: inner, held: held}, nil
}

// leaseName returns the database lease name of scope.
func leaseName(scope coord.Scope) string {
	return "coord\x00" + scope.VolumeID + "\x00" + scope.ObjectStoreID + "\x00" + scope.Key
}

// _ is a type assertion
var _ coord.Coordinator = (*Coordinator)(nil)
