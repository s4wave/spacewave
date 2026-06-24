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
	w.c.mu.Lock()
	defer w.c.mu.Unlock()

	select {
	case <-w.done:
		return nil
	default:
		close(w.done)
	}

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
