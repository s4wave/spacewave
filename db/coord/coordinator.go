package coord

import "context"

// Coordinator provides backend-neutral coordination for one Volume.
type Coordinator interface {
	// Capability reports whether the scoped ObjectStore supports direct
	// multi-writer coordination and why callers must fall back when it does not.
	Capability(ctx context.Context, scope Scope) (*Capability, error)
	// Snapshot returns the latest durable generation and root metadata.
	Snapshot(ctx context.Context, scope Scope) (*Snapshot, error)
	// Watch returns root, prefix, lock, and fallback events after generation.
	Watch(ctx context.Context, scope Scope, afterGeneration uint64) (Watch, error)
	// TryAcquireWriteLease attempts to acquire the logical write lease.
	TryAcquireWriteLease(ctx context.Context, scope Scope) (WriteLease, bool, error)
	// WaitAcquireWriteLease waits until the logical write lease is available.
	WaitAcquireWriteLease(ctx context.Context, scope Scope) (WriteLease, error)
}

// WriteLease owns one logical write turn for a scoped ObjectStore.
type WriteLease interface {
	// Refresh returns the latest durable generation and root before writing.
	Refresh(ctx context.Context) (*Snapshot, error)
	// Publish records an accepted generation/root/prefix change.
	Publish(ctx context.Context, event Event) (*Snapshot, error)
	// Release releases the logical write lease.
	Release(ctx context.Context) error
}

// Watch streams coordination events until closed or until its context ends.
type Watch interface {
	// Events returns the event stream for this watch.
	Events() <-chan Event
	// Close releases resources held by this watch.
	Close() error
}
