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
	once                    sync.Once
	refreshed               bool
	baseGeneration          uint64
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
	snapshot.Generation = l.c.generation()
	l.refreshed = true
	l.baseGeneration = snapshot.Generation
	return snapshot, nil
}

func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	if _, err := l.inner.Refresh(ctx); err != nil {
		return nil, err
	}

	generation := l.c.generation()
	if l.refreshed && generation != l.baseGeneration {
		l.baseGeneration = generation
	}
	event.Generation = generation
	snapshot, err := l.inner.Publish(ctx, event)
	if err != nil {
		return nil, err
	}
	snapshot.Generation = l.c.generation()
	return snapshot, nil
}

func (l *lease) Release(ctx context.Context) error {
	var releaseErr error
	l.once.Do(func() {
		if l.releaseCoordinationLock != nil {
			releaseErr = l.releaseCoordinationLock()
		}
		releaseErr = errors.Join(releaseErr, l.inner.Release(context.Background()))
	})
	return releaseErr
}

// _ is a type assertion
var _ coord.WriteLease = (*lease)(nil)
