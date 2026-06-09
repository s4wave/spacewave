//go:build js

package opfs

import (
	"context"

	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	"github.com/s4wave/spacewave/db/opfs/filelock"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/metashard"
)

// Coordinator adapts OPFS Web Locks, metashard generations, and
// BroadcastChannel invalidations into the Volume coordinator contract.
type Coordinator struct {
	meta          *metashard.MetaShard
	inner         *coord_inmem.Coordinator
	writeLockName string
	blockScope    string
}

// NewCoordinator builds an OPFS-backed coordinator.
func NewCoordinator(meta *metashard.MetaShard, lockPrefix string, inner *coord_inmem.Coordinator) *Coordinator {
	if inner == nil {
		inner = coord_inmem.NewCoordinator()
	}
	return &Coordinator{
		meta:          meta,
		inner:         inner,
		writeLockName: lockPrefix + "/coord/write",
		blockScope:    lockPrefix + "/blocks",
	}
}

// Capability reports OPFS coordination support.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	generation, err := c.generation(ctx)
	if err != nil {
		return nil, err
	}
	return &coord.Capability{
		Supported:     true,
		Backend:       coord.BackendKindOPFS,
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		Generation:    generation,
	}, nil
}

// Snapshot returns the latest metashard generation and coordinator root.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	snapshot, err := c.inner.Snapshot(ctx, scope)
	if err != nil {
		return nil, err
	}
	snapshot.Generation, err = c.generation(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// Watch streams root/prefix lease events and OPFS BroadcastChannel wakeups.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	inner, err := c.inner.Watch(ctx, scope, afterGeneration)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	w := &watch{
		ctx:      ctx,
		cancel:   cancel,
		c:        c,
		scope:    scope,
		inner:    inner,
		listener: blockshard.NewListener(c.blockScope),
		events:   make(chan coord.Event, 16),
		done:     make(chan struct{}),
	}
	w.start()
	return w, nil
}

// TryAcquireWriteLease attempts to acquire the OPFS logical write lease.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	releaseWebLock, acquired, err := filelock.AcquireWebLockIfAvailable(c.writeLockName, true)
	if err != nil || !acquired {
		return nil, acquired, err
	}

	inner, ok, err := c.inner.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		releaseWebLock()
		return nil, ok, err
	}
	return &lease{c: c, scope: scope, inner: inner, releaseWebLock: releaseWebLock}, true, nil
}

// WaitAcquireWriteLease waits until the OPFS logical write lease is available.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	releaseWebLock, err := filelock.AcquireWebLockContext(ctx, c.writeLockName, true)
	if err != nil {
		return nil, err
	}

	inner, err := c.inner.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		releaseWebLock()
		return nil, err
	}
	return &lease{c: c, scope: scope, inner: inner, releaseWebLock: releaseWebLock}, nil
}

func (c *Coordinator) generation(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return c.meta.RefreshGenerationContext(ctx)
}

var _ coord.Coordinator = (*Coordinator)(nil)
