//go:build !js && !wasip1

package bolt

import (
	"context"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

// Coordinator adapts bbolt commit generation into the Volume coordinator contract.
type Coordinator struct {
	db    *bdb.DB
	inner *coord_inmem.Coordinator
}

// NewCoordinator builds a bbolt-backed coordinator.
func NewCoordinator(db *bdb.DB, inner *coord_inmem.Coordinator) *Coordinator {
	if inner == nil {
		inner = coord_inmem.NewCoordinator()
	}
	return &Coordinator{
		db:    db,
		inner: inner,
	}
}

// Capability reports bbolt coordination support.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &coord.Capability{
		Supported:     true,
		Backend:       coord.BackendKindBbolt,
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		Generation:    c.generation(),
	}, nil
}

// Snapshot returns the latest bbolt commit generation and coordinator root.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	snapshot, err := c.inner.Snapshot(ctx, scope)
	if err != nil {
		return nil, err
	}
	snapshot.Generation = c.generation()
	return snapshot, nil
}

// Watch streams root/prefix lease events and bbolt commit-generation changes.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	inner, err := c.inner.Watch(ctx, scope, afterGeneration)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	w := &watch{
		ctx:    ctx,
		cancel: cancel,
		c:      c,
		scope:  scope,
		inner:  inner,
		events: make(chan coord.Event, 16),
		done:   make(chan struct{}),
	}
	w.start(afterGeneration)
	return w, nil
}

// TryAcquireWriteLease attempts to acquire the logical bbolt write lease.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	inner, ok, err := c.inner.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		return nil, ok, err
	}

	releaseCoordinationLock, acquired, err := c.tryAcquireCoordinationLock()
	if err != nil || !acquired {
		_ = inner.Release(context.Background())
		return nil, acquired, err
	}
	return &lease{c: c, scope: scope, inner: inner, releaseCoordinationLock: releaseCoordinationLock}, true, nil
}

// WaitAcquireWriteLease waits until the logical bbolt write lease is available.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
		if err != nil {
			return nil, err
		}
		if ok {
			return lease, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Coordinator) tryAcquireCoordinationLock() (func() error, bool, error) {
	if c == nil || c.db == nil {
		return func() error { return nil }, true, nil
	}
	acquired, err := c.db.TryAcquireCoordinationLock()
	if err != nil || !acquired {
		return nil, acquired, err
	}
	return c.db.ReleaseCoordinationLock, true, nil
}

func (c *Coordinator) generation() uint64 {
	generation, ok := c.safeGeneration()
	if !ok {
		return 0
	}
	return generation
}

func (c *Coordinator) safeGeneration() (generation uint64, ok bool) {
	if c == nil || c.db == nil {
		return 0, false
	}
	// CommitCounter panics on a closed or unopened bbolt DB; treat that
	// foreign panic as an unavailable generation rather than a crash.
	defer func() {
		if recover() != nil {
			generation = 0
			ok = false
		}
	}()
	return c.db.CommitCounter(), true
}

// _ is a type assertion
var _ coord.Coordinator = (*Coordinator)(nil)
