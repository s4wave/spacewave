package coord

import "context"

// UnsupportedCoordinator reports explicit proxy/RPC fallback for a Volume.
type UnsupportedCoordinator struct {
	// Backend is the unsupported backend kind to report.
	Backend BackendKind
	// Reason is the fallback reason to report.
	Reason FallbackReason
}

// NewUnsupportedCoordinator builds a Coordinator for unsupported backends.
func NewUnsupportedCoordinator(backend BackendKind, reason FallbackReason) *UnsupportedCoordinator {
	return &UnsupportedCoordinator{
		Backend: backend,
		Reason:  reason,
	}
}

// Capability reports an unsupported coordination capability.
func (c *UnsupportedCoordinator) Capability(ctx context.Context, scope Scope) (*Capability, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	backend := c.Backend
	if backend == BackendKindUnknown {
		backend = BackendKindUnsupported
	}
	reason := c.Reason
	if reason == FallbackReasonNone {
		reason = FallbackReasonUnsupported
	}

	return &Capability{
		Supported:      false,
		Backend:        backend,
		VolumeID:       scope.VolumeID,
		ObjectStoreID:  scope.ObjectStoreID,
		FallbackReason: reason,
	}, nil
}

// Snapshot returns ErrUnsupported because no durable coordination snapshot exists.
func (c *UnsupportedCoordinator) Snapshot(ctx context.Context, scope Scope) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, ErrUnsupported
}

// Watch returns ErrUnsupported because the backend has no direct event stream.
func (c *UnsupportedCoordinator) Watch(ctx context.Context, scope Scope, afterGeneration uint64) (Watch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, ErrUnsupported
}

// TryAcquireWriteLease returns ErrUnsupported because no direct lease exists.
func (c *UnsupportedCoordinator) TryAcquireWriteLease(ctx context.Context, scope Scope) (WriteLease, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return nil, false, ErrUnsupported
}

// WaitAcquireWriteLease returns ErrUnsupported because no direct lease exists.
func (c *UnsupportedCoordinator) WaitAcquireWriteLease(ctx context.Context, scope Scope) (WriteLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, ErrUnsupported
}

// _ is a type assertion
var _ Coordinator = (*UnsupportedCoordinator)(nil)
