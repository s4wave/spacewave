//go:build !js && !wasip1

package bolt

import (
	"context"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

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
	// Reject canceled requests before reading coordinator state.
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
	// Read the inner snapshot before overlaying the bbolt generation.
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
	// Start the inner watch before creating the bbolt event stream.
	inner, err := c.inner.Watch(ctx, scope, afterGeneration)
	if err != nil {
		return nil, err
	}

	// Create and start the combined watch lifecycle.
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
	// Reject canceled requests before reserving the write lease.
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if scope.Key != "" {
		// Keyed scopes coordinate through the inner in-memory coordinator: the
		// bbolt coordination lock is the single per-DB write turn, not a keyed
		// exclusion namespace.
		return c.inner.TryAcquireWriteLease(ctx, scope)
	}

	// Reserve the local write turn before acquiring the inner lease.
	if reserved, _ := c.reserveWriteLease(); !reserved {
		return nil, false, nil
	}

	// Roll back local reservation when inner or cross-process acquisition fails.
	inner, ok, err := c.inner.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		c.releaseWriteLease()
		return nil, ok, err
	}

	// Acquire the cross-process write turn or release the local reservation.
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
	// Keyed scopes coordinate through the inner coordinator.
	if scope.Key != "" {
		return c.inner.WaitAcquireWriteLease(ctx, scope)
	}

	// Reserve this process's write turn, waiting for a local holder to release it.
	if err := c.waitReserveWriteLease(ctx); err != nil {
		return nil, err
	}

	// Acquire the inner lease after reserving this process's write turn.
	inner, err := c.inner.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		c.releaseWriteLease()
		return nil, err
	}

	// Wait for other processes to release the bbolt lock.
	releaseCoordinationLock, err := c.waitCoordinationLock(ctx, func() {
		_ = inner.Release(context.Background())
		c.releaseWriteLease()
	})
	if err != nil {
		return nil, err
	}
	return &lease{c: c, scope: scope, inner: inner, releaseCoordinationLock: releaseCoordinationLock}, nil
}

// waitReserveWriteLease reserves this process's write turn, waiting for its
// release notification while another local lease holds it.
func (c *Coordinator) waitReserveWriteLease(ctx context.Context) error {
	for {
		reserved, waitCh := c.reserveWriteLease()
		if reserved {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
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

// waitCoordinationLock blocks in the kernel until this process holds the bbolt
// coordination lock, returning its release function. On failure it calls
// releaseTurn. If ctx ends first, it returns ctx.Err() and a goroutine calls
// releaseTurn once the blocked acquire returns: the lock belongs to the
// process, so the local write turn must stay reserved until then or a second
// waiter would share the grant.
func (c *Coordinator) waitCoordinationLock(ctx context.Context, releaseTurn func()) (func() error, error) {
	// A coordinator without a database has no other process to wait for.
	if c == nil || c.db == nil {
		return func() error { return nil }, nil
	}

	// Acquire in the background so the wait can follow ctx.
	acquired := make(chan error, 1)
	go func() { acquired <- c.db.AcquireCoordinationLock() }()
	select {
	case err := <-acquired:
		if err != nil {
			releaseTurn()
			return nil, err
		}
		return c.db.ReleaseCoordinationLock, nil
	case <-ctx.Done():
	}

	// Release the lock and the turn once the abandoned acquire returns.
	go func() {
		if err := <-acquired; err == nil {
			_ = c.db.ReleaseCoordinationLock()
		}
		releaseTurn()
	}()
	return nil, ctx.Err()
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
