//go:build js

package opfs

import (
	"context"

	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	db_opfs "github.com/s4wave/spacewave/db/opfs"
)

// GenerationSource reads the latest committed storage generation.
type GenerationSource interface {
	// RefreshGenerationContext reads the authoritative logical revision.
	RefreshGenerationContext(context.Context) (uint64, error)
	// WaitGeneration waits for a later durable logical revision or cancellation.
	WaitGeneration(context.Context, uint64) (uint64, error)
}

// Coordinator adapts OPFS Web Locks, committed generations, and
// BroadcastChannel invalidations into the Volume coordinator contract.
type Coordinator struct {
	// meta owns durable logical revisions and their notification lifetime.
	meta GenerationSource
	// inner owns local logical leases and detailed prefix/root events.
	inner *coord_inmem.Coordinator
	// lockPrefix scopes logical write leases to the mounted volume.
	lockPrefix string
}

// NewCoordinator builds an OPFS-backed coordinator. source may be nil when the
// inner coordinator carries generations alone.
func NewCoordinator(source GenerationSource, lockPrefix string, inner *coord_inmem.Coordinator) *Coordinator {
	if inner == nil {
		inner = coord_inmem.NewCoordinator()
	}
	return &Coordinator{
		meta:       source,
		inner:      inner,
		lockPrefix: lockPrefix,
	}
}

// Capability reports OPFS coordination support.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	// Reject canceled capability requests before reading generation state.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Build the capability record and mark keyed scopes as generation-free.
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

	// Read the committed generation for root scopes.
	generation, err := c.generation(ctx, scope)
	if err != nil {
		return nil, err
	}
	capability.Generation = generation
	return capability, nil
}

// Snapshot returns the latest committed generation and coordinator root.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	// Read the inner snapshot before overlaying the committed generation.
	snapshot, err := c.inner.Snapshot(ctx, scope)
	if err != nil {
		return nil, err
	}
	if c.meta == nil {
		return snapshot, nil
	}

	// Refresh the generation only when this coordinator has a committed source.
	snapshot.Generation, err = c.generation(ctx, scope)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// Watch streams root/prefix lease events and OPFS BroadcastChannel wakeups.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	// Start the inner watch before attaching OPFS invalidation events.
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
		after:  afterGeneration,
		events: make(chan coord.Event, 16),
		done:   make(chan struct{}),
	}
	w.start()
	return w, nil
}

// TryAcquireWriteLease attempts to acquire the OPFS logical write lease.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	// Reject canceled requests before acquiring the Web Lock and inner lease.
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	// Acquire the Web Lock before claiming the inner logical lease.
	webLock, err := db_opfs.DefaultDriver.AcquireWebLockIfAvailable(ctx, writeLockName(c.lockPrefix, scope), true)
	if err != nil || webLock.Outcome != db_opfs.WebLockOutcomeAcquired {
		return nil, false, err
	}

	// Claim the inner lease and release Web Lock state on failure.
	inner, ok, err := c.inner.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		webLock.Release()
		return nil, ok, err
	}
	return &lease{c: c, scope: scope, inner: inner, webLock: webLock}, true, nil
}

// WaitAcquireWriteLease waits until the OPFS logical write lease is available.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	// Acquire the inner lease before waiting for the Web Lock.
	inner, err := c.inner.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Acquire the Web Lock and roll back the inner lease on failure.
	webLock, err := db_opfs.DefaultDriver.AcquireWebLock(ctx, writeLockName(c.lockPrefix, scope), true)
	if err == nil && webLock.Outcome != db_opfs.WebLockOutcomeAcquired {
		err = errors.Errorf("write lease Web Lock request ended with outcome %d", webLock.Outcome)
	}
	if err != nil {
		_ = inner.Release(context.Background())
		return nil, err
	}
	return &lease{c: c, scope: scope, inner: inner, webLock: webLock}, nil
}

// generation reads the durable logical revision or the standalone local source.
func (c *Coordinator) generation(ctx context.Context, scope coord.Scope) (uint64, error) {
	// Reject canceled generation reads before consulting committed state.
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
