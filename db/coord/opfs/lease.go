//go:build js

package opfs

import (
	"context"
	"sync"
	"syscall/js"

	"github.com/s4wave/spacewave/db/coord"
)

type lease struct {
	c              *Coordinator
	scope          coord.Scope
	inner          coord.WriteLease
	releaseWebLock func()
	mtx            sync.Mutex
	released       bool
	releaseErr     error
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

func (l *lease) Release(ctx context.Context) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.released {
		return l.releaseErr
	}
	l.released = true
	l.releaseWebLock()
	waitForWebLockRelease()
	l.releaseErr = l.inner.Release(context.Background())
	return l.releaseErr
}

func waitForWebLockRelease() {
	done := make(chan struct{})
	callback := js.FuncOf(func(js.Value, []js.Value) any {
		close(done)
		return nil
	})
	defer callback.Release()
	js.Global().Call("queueMicrotask", callback)
	<-done
}

// _ is a type assertion
var _ coord.WriteLease = (*lease)(nil)
