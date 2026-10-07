package volume_scoped

import (
	"strings"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
)

// watch strips the view prefix from the object store ID of each event of a
// coordinator watch and drops the events of object stores outside the view.
type watch struct {
	// inner is the watch on the underlying volume.
	inner coord.Watch
	// events carries the mapped events until inner ends.
	events chan coord.Event
	// done is closed by Close to stop the forwarding goroutine.
	done chan struct{}
	// closeOnce guards Close.
	closeOnce sync.Once
}

// newWatch forwards the events of inner with prefix stripped from their object
// store IDs.
func newWatch(inner coord.Watch, prefix string) *watch {
	w := &watch{
		inner:  inner,
		events: make(chan coord.Event),
		done:   make(chan struct{}),
	}
	go w.forward(prefix)
	return w
}

// Events returns the mapped event stream. It closes when the inner watch ends.
func (w *watch) Events() <-chan coord.Event {
	return w.events
}

// Close closes the inner watch and stops forwarding.
func (w *watch) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.done)
		err = w.inner.Close()
	})
	return err
}

// forward maps each inner event until the inner stream or the watch ends.
func (w *watch) forward(prefix string) {
	defer close(w.events)
	for event := range w.inner.Events() {
		// Drop events of object stores outside the view.
		id, ok := strings.CutPrefix(event.ObjectStoreID, prefix)
		if !ok {
			continue
		}

		event.ObjectStoreID = id
		select {
		case w.events <- event:
		case <-w.done:
			return
		}
	}
}

// _ is a type assertion
var _ coord.Watch = (*watch)(nil)
