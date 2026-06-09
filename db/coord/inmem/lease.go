package inmem

import (
	"context"

	"github.com/s4wave/spacewave/db/coord"
)

type lease struct {
	c        *Coordinator
	scope    coord.Scope
	state    *scopeState
	released bool
}

func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	l.c.mu.Lock()
	defer l.c.mu.Unlock()

	if l.released || l.state != l.c.getScopeLocked(l.scope) || !l.state.locked {
		return nil, coord.ErrLeaseReleased
	}
	return l.state.snapshot(l.scope), nil
}

func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	l.c.mu.Lock()
	defer l.c.mu.Unlock()

	if l.released || l.state != l.c.getScopeLocked(l.scope) || !l.state.locked {
		return nil, coord.ErrLeaseReleased
	}

	event.ProcessID = l.scope.ParticipantID
	event.VolumeID = l.scope.VolumeID
	event.ObjectStoreID = l.scope.ObjectStoreID
	if event.Generation <= l.state.generation {
		event.Generation = l.state.generation + 1
	}
	l.state.generation = event.Generation
	if event.RootChanged != nil {
		l.state.root = event.RootChanged.Clone()
	}
	l.state.publishLocked(event)

	return l.state.snapshot(l.scope), nil
}

func (l *lease) Release(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	l.c.mu.Lock()
	defer l.c.mu.Unlock()

	if l.released {
		return nil
	}
	l.released = true
	if l.state == l.c.getScopeLocked(l.scope) && l.state.locked {
		l.state.locked = false
		l.state.publishLocked(coord.Event{
			ProcessID:     l.scope.ParticipantID,
			VolumeID:      l.scope.VolumeID,
			ObjectStoreID: l.scope.ObjectStoreID,
			Generation:    l.state.generation,
			Unlocked:      true,
		})
		l.c.cond.Broadcast()
	}

	return nil
}

var _ coord.WriteLease = (*lease)(nil)
