//go:build !js && !wasip1

package bolt

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
)

type watch struct {
	ctx        context.Context
	cancel     context.CancelFunc
	c          *Coordinator
	scope      coord.Scope
	inner      coord.Watch
	events     chan coord.Event
	done       chan struct{}
	commitDone chan struct{}
	once       sync.Once
}

func (w *watch) Events() <-chan coord.Event {
	return w.events
}

func (w *watch) Close() error {
	var err error
	w.once.Do(func() {
		w.cancel()
		err = w.inner.Close()
		<-w.done
		<-w.commitDone
	})
	return err
}

func (w *watch) start(afterGeneration uint64) {
	commitEvents := w.watchCommits(afterGeneration)
	go func() {
		defer close(w.done)
		defer close(w.events)

		innerEvents := w.inner.Events()
		for {
			select {
			case <-w.ctx.Done():
				return
			case event, ok := <-innerEvents:
				if !ok {
					innerEvents = nil
					continue
				}
				w.send(event)
			case event, ok := <-commitEvents:
				if !ok {
					commitEvents = nil
					continue
				}
				w.send(event)
			}
			if innerEvents == nil && commitEvents == nil {
				return
			}
		}
	}()
}

func (w *watch) watchCommits(afterGeneration uint64) <-chan coord.Event {
	ch := make(chan coord.Event, 1)
	w.commitDone = make(chan struct{})
	go func() {
		defer close(w.commitDone)
		defer close(ch)
		last := afterGeneration
		for {
			next, err := w.waitCommitCounter(last)
			if err != nil {
				return
			}
			if next <= last {
				return
			}
			last = next
			select {
			case <-w.ctx.Done():
				return
			case ch <- coord.Event{
				VolumeID:      w.scope.VolumeID,
				ObjectStoreID: w.scope.ObjectStoreID,
				Generation:    next,
			}:
			}
		}
	}()
	return ch
}

func (w *watch) waitCommitCounter(last uint64) (next uint64, err error) {
	// WaitCommitCounter panics on a closed bbolt DB; treat that foreign panic
	// as cancellation so the commit watcher exits cleanly.
	defer func() {
		if recover() != nil {
			err = context.Canceled
		}
	}()
	return w.c.db.WaitCommitCounter(w.ctx, last)
}

func (w *watch) send(event coord.Event) {
	if event.Generation == 0 {
		event.Generation = w.c.generation()
	}
	select {
	case <-w.ctx.Done():
	case w.events <- event:
	}
}

// _ is a type assertion
var _ coord.Watch = (*watch)(nil)
