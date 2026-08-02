//go:build !js && !wasip1

package bolt

import (
	"context"
	"errors"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
)

type lease struct {
	c                       *Coordinator
	scope                   coord.Scope
	inner                   coord.WriteLease
	releaseCoordinationLock func() error
	mtx                     sync.Mutex
	released                bool
	releaseErr              error
	refreshed               bool
	baseGeneration          uint64
}

// Done returns the inner lease channel, closed when Release completes the
// inner release. The bbolt lease cannot be lost involuntarily.
func (l *lease) Done() <-chan struct{} {
	return l.inner.Done()
}

// Err returns the inner lease error, nil for a held or cleanly released lease.
func (l *lease) Err() error {
	return l.inner.Err()
}

func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	if l.c != nil && l.c.db != nil {
		if err := l.c.db.RefreshForCoordinationLock(); err != nil {
			return nil, err
		}
	}
	snapshot, err := l.inner.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	if generation, ok := l.c.safeGeneration(); ok {
		snapshot.Generation = generation
		l.refreshed = true
		l.baseGeneration = generation
	}
	return snapshot, nil
}

func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	if _, err := l.inner.Refresh(ctx); err != nil {
		return nil, err
	}

	generation, durable := l.c.safeGeneration()
	if durable {
		if l.refreshed && generation != l.baseGeneration {
			l.baseGeneration = generation
		}
		event.Generation = generation
	}
	snapshot, err := l.inner.Publish(ctx, event)
	if err != nil {
		return nil, err
	}
	if durable {
		snapshot.Generation = generation
	}
	return snapshot, nil
}

func (l *lease) Release(ctx context.Context) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.released {
		return l.releaseErr
	}
	l.released = true
	if l.releaseCoordinationLock != nil {
		l.releaseErr = l.releaseCoordinationLock()
	}
	l.releaseErr = errors.Join(l.releaseErr, l.inner.Release(context.Background()))
	l.c.releaseWriteLease()
	return l.releaseErr
}

// _ is a type assertion
var _ coord.WriteLease = (*lease)(nil)
