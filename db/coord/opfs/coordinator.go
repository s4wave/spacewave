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
	meta       *metashard.MetaShard
	inner      *coord_inmem.Coordinator
	lockPrefix string
	blockScope string
}

// NewCoordinator builds an OPFS-backed coordinator. meta may be nil for
// backends with Web Lock exclusion but no metashard generation store; the
// inner coordinator then carries generations alone.
func NewCoordinator(meta *metashard.MetaShard, lockPrefix string, inner *coord_inmem.Coordinator) *Coordinator {
	if inner == nil {
		inner = coord_inmem.NewCoordinator()
	}
	return &Coordinator{
		meta:       meta,
		inner:      inner,
		lockPrefix: lockPrefix,
		blockScope: lockPrefix + "/blocks",
	}
}

// Capability reports OPFS coordination support.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	capability := &coord.Capability{
		Supported:     true,
		Backend:       coord.BackendKindOPFS,
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		Generations:   scope.Key == "",
	}
	if !capability.Generations {
		return capability, nil
	}
	generation, err := c.generation(ctx, scope)
	if err != nil {
		return nil, err
	}
	capability.Generation = generation
	return capability, nil
}

// Snapshot returns the latest metashard generation and coordinator root.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	snapshot, err := c.inner.Snapshot(ctx, scope)
	if err != nil {
		return nil, err
	}
	if c.meta == nil {
		return snapshot, nil
	}
	snapshot.Generation, err = c.generation(ctx, scope)
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

	releaseWebLock, acquired, err := filelock.AcquireWebLockIfAvailable(writeLockName(c.lockPrefix, scope), true)
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
	inner, err := c.inner.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return nil, err
	}

	releaseWebLock, err := filelock.AcquireWebLockContext(ctx, writeLockName(c.lockPrefix, scope), true)
	if err != nil {
		_ = inner.Release(context.Background())
		return nil, err
	}
	return &lease{c: c, scope: scope, inner: inner, releaseWebLock: releaseWebLock}, nil
}

func (c *Coordinator) generation(ctx context.Context, scope coord.Scope) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if c.meta == nil {
		snapshot, err := c.inner.Snapshot(ctx, scope)
		if err != nil {
			return 0, err
		}
		return snapshot.Generation, nil
	}
	return c.meta.RefreshGenerationContext(ctx)
}

// _ is a type assertion
var _ coord.Coordinator = (*Coordinator)(nil)
