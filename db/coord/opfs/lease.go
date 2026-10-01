//go:build js

package opfs

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
	db_opfs "github.com/s4wave/spacewave/db/opfs"
)

// lease is a logical write lease held under an exclusive Web Lock.
type lease struct {
	// c is the coordinator that granted the lease.
	c *Coordinator
	// scope is the leased write scope.
	scope coord.Scope
	// inner is the local logical lease.
	inner coord.WriteLease
	// webLock is the exclusive Web Lock that spans contexts.
	webLock *db_opfs.WebLockResult

	// mtx guards released and releaseErr.
	mtx sync.Mutex
	// released is set by the first Release.
	released bool
	// releaseErr is the result of the first Release.
	releaseErr error
}

// Done returns the inner lease channel, closed when Release completes the
// inner release. The Web Lock lease cannot report involuntary loss.
func (l *lease) Done() <-chan struct{} {
	return l.inner.Done()
}

// Err returns the inner lease error, nil for a held or cleanly released lease.
func (l *lease) Err() error {
	return l.inner.Err()
}

func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	snapshot, err := l.inner.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	if l.c.meta != nil {
		snapshot.Generation, err = l.c.generation(ctx, l.scope)
		if err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	if _, err := l.inner.Refresh(ctx); err != nil {
		return nil, err
	}

	if l.c.meta == nil {
		return l.inner.Publish(ctx, event)
	}
	generation, err := l.c.generation(ctx, l.scope)
	if err != nil {
		return nil, err
	}
	event.Generation = generation
	snapshot, err := l.inner.Publish(ctx, event)
	if err != nil {
		return nil, err
	}
	snapshot.Generation, err = l.c.generation(ctx, l.scope)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// Release frees the Web Lock and the inner lease. It returns after the browser
// frees the Web Lock, so another context can acquire the lease at once. A
// canceled ctx does not stop the release.
func (l *lease) Release(ctx context.Context) error {
	// Release once, returning the first result on later calls.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.released {
		return l.releaseErr
	}
	l.released = true

	// Free the Web Lock and wait until the browser has freed it.
	l.webLock.Release()
	<-l.webLock.Released

	// Release the local logical lease.
	l.releaseErr = l.inner.Release(context.Background())
	return l.releaseErr
}

// _ is a type assertion
var _ coord.WriteLease = (*lease)(nil)
