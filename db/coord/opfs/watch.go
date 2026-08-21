//go:build js

package opfs

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
)

type watch struct {
	ctx      context.Context
	cancel   context.CancelFunc
	c        *Coordinator
	scope    coord.Scope
	inner    coord.Watch
	listener *blockshard.Listener
	events   chan coord.Event
	done     chan struct{}
	once     atomic.Bool
}

func (w *watch) Events() <-chan coord.Event {
	return w.events
}

func (w *watch) Close() error {
	var err error
	if w.once.CompareAndSwap(false, true) {
		w.cancel()
		err = w.inner.Close()
		w.listener.Close()
		<-w.done
	}
	return err
}

func (w *watch) start() {
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
			case <-w.listener.Notify():
				w.listener.DrainPending()
				w.send(coord.Event{
					VolumeID:      w.scope.VolumeID,
					ObjectStoreID: w.scope.ObjectStoreID,
				})
			}
			if innerEvents == nil {
				return
			}
		}
	}()
}

func (w *watch) send(event coord.Event) {
	if event.Generation == 0 {
		generation, err := w.c.generation(w.ctx, w.scope)
		if err != nil {
			return
		}
		event.Generation = generation
	}
	select {
	case <-w.ctx.Done():
	case w.events <- event:
	}
}

// _ is a type assertion
var _ coord.Watch = (*watch)(nil)
