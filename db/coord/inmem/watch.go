package inmem

import "github.com/s4wave/spacewave/db/coord"

type watch struct {
	c     *Coordinator
	scope coord.Scope
	id    uint64
	ch    chan coord.Event
	done  chan struct{}
}

func (w *watch) Events() <-chan coord.Event {
	return w.ch
}

func (w *watch) Close() error {
	// Hold the coordinator lock while detaching the scope watcher.
	w.c.mu.Lock()
	defer w.c.mu.Unlock()

	// Close the watcher's lifecycle once before removing its event channel.
	select {
	case <-w.done:
		return nil
	default:
		close(w.done)
	}

	// Remove the watcher from the scope and finish its event stream.
	state := w.c.getScopeLocked(w.scope)
	delete(state.watchers, w.id)
	close(w.ch)
	return nil
}

func (w *watch) sendLocked(event coord.Event) {
	select {
	case <-w.done:
	case w.ch <- event:
	default:
	}
}

// _ is a type assertion
var _ coord.Watch = (*watch)(nil)
