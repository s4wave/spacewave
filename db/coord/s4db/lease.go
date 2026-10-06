package s4db

import (
	"context"
	"errors"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
	db_s4db "github.com/s4wave/spacewave/db/s4db"
)

// lease joins this process's inner lease with the scope's database lease.
type lease struct {
	// db is the database file.
	db *db_s4db.DB
	// scope is the leased scope.
	scope coord.Scope
	// inner is this process's turn, which holds the root.
	inner coord.WriteLease
	// held excludes other processes.
	held *db_s4db.Lease

	// mtx guards released and releaseErr.
	mtx        sync.Mutex
	released   bool
	releaseErr error
}

// Done returns the inner lease channel, closed by Release. The system keeps a
// database lease until the process releases it or dies, so the lease is never
// lost while the process runs.
func (l *lease) Done() <-chan struct{} {
	return l.inner.Done()
}

// Err returns the inner lease error, nil for a held or cleanly released lease.
func (l *lease) Err() error {
	return l.inner.Err()
}

// Refresh applies the commits other processes wrote and returns the inner root
// at the resulting commit sequence.
func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	// Check that the lease is held and read the root.
	snapshot, err := l.inner.Refresh(ctx)
	if err != nil {
		return nil, err
	}

	// Apply the other processes' commits before reading the sequence.
	if err := l.db.Refresh(ctx); err != nil {
		return nil, err
	}
	snapshot.Generation = l.db.Seq()
	return snapshot, nil
}

// Publish records the event at the current commit sequence.
func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	// Keyed scopes carry no generations.
	if l.scope.Key != "" {
		return nil, coord.ErrUnsupported
	}

	// Publish through the inner lease at the current commit sequence.
	generation := l.db.Seq()
	event.Generation = generation
	snapshot, err := l.inner.Publish(ctx, event)
	if err != nil {
		return nil, err
	}
	snapshot.Generation = generation
	return snapshot, nil
}

// Release releases the database lease, then the inner lease, so a woken local
// waiter finds the scope free in the file.
func (l *lease) Release(context.Context) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if !l.released {
		l.released = true
		l.releaseErr = errors.Join(l.held.Release(), l.inner.Release(context.Background()))
	}
	return l.releaseErr
}

// _ is a type assertion
var _ coord.WriteLease = (*lease)(nil)
