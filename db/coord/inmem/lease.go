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
	done     chan struct{}
}

// Done returns a channel closed by Release. The in-memory lease cannot be
// lost involuntarily, so Done never closes while the lease is held.
func (l *lease) Done() <-chan struct{} {
	return l.done
}

// Err returns nil: the in-memory lease is only ever released cleanly.
func (*lease) Err() error {
	return nil
}

func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.scope.Key != "" {
		return nil, coord.ErrUnsupported
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
	if l.scope.Key != "" {
		return nil, coord.ErrUnsupported
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

// Release drops the lease and wakes waiters. It ignores its context: the
// cleanup path that releases after a canceled write is the one that most needs
// the scope unlocked.
func (l *lease) Release(context.Context) error {
	l.c.mu.Lock()
	defer l.c.mu.Unlock()

	if l.released {
		return nil
	}
	l.released = true
	close(l.done)
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

// _ is a type assertion
var _ coord.WriteLease = (*lease)(nil)
