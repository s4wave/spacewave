package volume_scoped

import (
	"context"

	"github.com/s4wave/spacewave/db/coord"
)

// writeLease maps the object store IDs of the events and snapshots that pass
// through a write lease of the underlying volume.
type writeLease struct {
	// inner is the lease on the underlying volume.
	inner coord.WriteLease
	// prefix is the view prefix.
	prefix string
}

// Done returns a channel closed when the lease is released or lost.
func (l *writeLease) Done() <-chan struct{} {
	return l.inner.Done()
}

// Err returns the loss error after Done is closed.
func (l *writeLease) Err() error {
	return l.inner.Err()
}

// Refresh returns the latest durable snapshot of the leased object store.
func (l *writeLease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	snapshot, err := l.inner.Refresh(ctx)
	return unscopeSnapshot(snapshot, l.prefix), err
}

// Publish records an accepted change in the leased object store.
func (l *writeLease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	event.ObjectStoreID = l.prefix + event.ObjectStoreID
	snapshot, err := l.inner.Publish(ctx, event)
	return unscopeSnapshot(snapshot, l.prefix), err
}

// Release releases the lease.
func (l *writeLease) Release(ctx context.Context) error {
	return l.inner.Release(ctx)
}

// _ is a type assertion
var _ coord.WriteLease = (*writeLease)(nil)
