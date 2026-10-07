package volume_scoped

import (
	"context"
	"strings"

	"github.com/s4wave/spacewave/db/coord"
)

// coordinator serves the coordination scopes of a scoped volume view.
//
// The object store ID and the exclusion key of every scope gain the view
// prefix, so the view coordinates only its own object stores.
type coordinator struct {
	// inner is the coordinator of the underlying volume.
	inner coord.Coordinator
	// prefix is the view prefix.
	prefix string
}

// Capability reports whether the scoped object store supports direct
// multi-writer coordination.
func (c *coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	// Read the capability of the prefixed scope.
	capability, err := c.inner.Capability(ctx, c.scopeScope(scope))
	if capability == nil {
		return nil, err
	}

	// Report the capability under the view's object store ID.
	unscoped := *capability
	unscoped.ObjectStoreID = strings.TrimPrefix(unscoped.ObjectStoreID, c.prefix)
	return &unscoped, err
}

// Snapshot returns the latest durable generation and root metadata.
func (c *coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	snapshot, err := c.inner.Snapshot(ctx, c.scopeScope(scope))
	return unscopeSnapshot(snapshot, c.prefix), err
}

// Watch returns root, prefix, lock, and fallback events after generation.
func (c *coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	inner, err := c.inner.Watch(ctx, c.scopeScope(scope), afterGeneration)
	if err != nil {
		return nil, err
	}
	return newWatch(inner, c.prefix), nil
}

// TryAcquireWriteLease attempts to acquire the logical write lease.
func (c *coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	lease, acquired, err := c.inner.TryAcquireWriteLease(ctx, c.scopeScope(scope))
	if err != nil || !acquired {
		return nil, acquired, err
	}
	return &writeLease{inner: lease, prefix: c.prefix}, true, nil
}

// WaitAcquireWriteLease waits until the logical write lease is available.
func (c *coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	lease, err := c.inner.WaitAcquireWriteLease(ctx, c.scopeScope(scope))
	if err != nil {
		return nil, err
	}
	return &writeLease{inner: lease, prefix: c.prefix}, nil
}

// scopeScope adds the view prefix to the object store ID and exclusion key of
// scope.
func (c *coordinator) scopeScope(scope coord.Scope) coord.Scope {
	scope.ObjectStoreID = c.prefix + scope.ObjectStoreID
	if scope.Key != "" {
		scope.Key = c.prefix + scope.Key
	}
	return scope
}

// unscopeSnapshot returns a copy of snapshot with the prefix removed from its
// object store ID. It returns nil for a nil snapshot.
func unscopeSnapshot(snapshot *coord.Snapshot, prefix string) *coord.Snapshot {
	if snapshot == nil {
		return nil
	}

	unscoped := *snapshot
	unscoped.ObjectStoreID = strings.TrimPrefix(unscoped.ObjectStoreID, prefix)
	return &unscoped
}

// _ is a type assertion
var _ coord.Coordinator = (*coordinator)(nil)
