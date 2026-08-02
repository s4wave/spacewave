//go:build !js && !wasip1

package bolt

import (
	"context"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

const crossProcessLockRetryDelay = 250 * time.Millisecond

// Coordinator adapts bbolt commit generation into the Volume coordinator contract.
type Coordinator struct {
	db    *bdb.DB
	inner *coord_inmem.Coordinator

	// bcast guards writeLocked.
	bcast broadcast.Broadcast
	// writeLocked indicates this process owns or is attempting the bbolt write lease.
	writeLocked bool
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
		Generations:   scope.Key == "",
	}, nil
}

// Snapshot returns the latest bbolt commit generation and coordinator root.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	snapshot, err := c.inner.Snapshot(ctx, scope)
	if err != nil {
		return nil, err
	}
	if generation, ok := c.safeGeneration(); ok {
		snapshot.Generation = generation
	}
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
	if scope.Key != "" {
		// Keyed scopes coordinate through the inner in-memory coordinator: the
		// bbolt coordination lock is the single per-DB write turn, not a keyed
		// exclusion namespace.
		return c.inner.TryAcquireWriteLease(ctx, scope)
	}
	if reserved, _ := c.reserveWriteLease(); !reserved {
		return nil, false, nil
	}

	inner, ok, err := c.inner.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		c.releaseWriteLease()
		return nil, ok, err
	}

	releaseCoordinationLock, acquired, err := c.tryAcquireCoordinationLock()
	if err != nil || !acquired {
		_ = inner.Release(context.Background())
		c.releaseWriteLease()
		return nil, acquired, err
	}
	return &lease{c: c, scope: scope, inner: inner, releaseCoordinationLock: releaseCoordinationLock}, true, nil
}

// WaitAcquireWriteLease waits until the logical bbolt write lease is available.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	if scope.Key != "" {
		return c.inner.WaitAcquireWriteLease(ctx, scope)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		reserved, waitCh := c.reserveWriteLease()
		if !reserved {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-waitCh:
			}
			continue
		}

		inner, err := c.inner.WaitAcquireWriteLease(ctx, scope)
		if err != nil {
			c.releaseWriteLease()
			return nil, err
		}

		releaseCoordinationLock, acquired, err := c.tryAcquireCoordinationLock()
		if err != nil {
			_ = inner.Release(context.Background())
			c.releaseWriteLease()
			return nil, err
		}
		if acquired {
			return &lease{c: c, scope: scope, inner: inner, releaseCoordinationLock: releaseCoordinationLock}, nil
		}

		_ = inner.Release(context.Background())
		c.releaseWriteLease()

		// bbolt exposes cross-process coordination as a non-blocking file-lock
		// probe; external processes cannot broadcast their release here.
		timer := time.NewTimer(crossProcessLockRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Coordinator) reserveWriteLease() (bool, <-chan struct{}) {
	var reserved bool
	var waitCh <-chan struct{}
	c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		if c.writeLocked {
			waitCh = getWaitCh()
			return
		}
		c.writeLocked = true
		reserved = true
	})
	return reserved, waitCh
}

func (c *Coordinator) releaseWriteLease() {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !c.writeLocked {
			return
		}
		c.writeLocked = false
		broadcast()
	})
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

func (c *Coordinator) safeGeneration() (uint64, bool) {
	if c == nil || c.db == nil {
		return 0, false
	}
	return c.db.CommitCounter(), true
}

// _ is a type assertion
var _ coord.Coordinator = (*Coordinator)(nil)
