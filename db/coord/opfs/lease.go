//go:build js

package opfs

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
)

type lease struct {
	c              *Coordinator
	scope          coord.Scope
	inner          coord.WriteLease
	releaseWebLock func()
	once           sync.Once
}

func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	snapshot, err := l.inner.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	snapshot.Generation, err = l.c.generation(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	if _, err := l.inner.Refresh(ctx); err != nil {
		return nil, err
	}

	generation, err := l.c.generation(ctx)
	if err != nil {
		return nil, err
	}
	event.Generation = generation
	snapshot, err := l.inner.Publish(ctx, event)
	if err != nil {
		return nil, err
	}
	snapshot.Generation, err = l.c.generation(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (l *lease) Release(ctx context.Context) error {
	err := l.inner.Release(context.Background())
	l.once.Do(l.releaseWebLock)
	return err
}

var _ coord.WriteLease = (*lease)(nil)
