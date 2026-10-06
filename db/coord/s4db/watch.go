package s4db

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
	db_s4db "github.com/s4wave/spacewave/db/s4db"
)

// watch merges the inner events with the database's commits.
type watch struct {
	// cancel stops both streams.
	cancel context.CancelFunc
	// inner is the inner watch.
	inner coord.Watch
	// events carries the merged stream, closed once both streams end.
	events chan coord.Event
	// done is closed after events.
	done chan struct{}
	// once closes the watch once.
	once sync.Once
	// err is the inner watch's close result.
	err error
}

// newWatch starts a watch of scope in db over inner, reporting commits after
// afterGeneration.
func newWatch(ctx context.Context, db *db_s4db.DB, scope coord.Scope, inner coord.Watch, afterGeneration uint64) *watch {
	// Start both streams.
	ctx, cancel := context.WithCancel(ctx)
	w := &watch{
		cancel: cancel,
		inner:  inner,
		events: make(chan coord.Event, 16),
		done:   make(chan struct{}),
	}
	var wg sync.WaitGroup
	wg.Go(func() { w.forward(ctx, db) })
	wg.Go(func() { w.commits(ctx, db, scope, afterGeneration) })

	// Close the stream once both end.
	go func() {
		wg.Wait()
		close(w.events)
		close(w.done)
	}()
	return w
}

// Events returns the merged event stream.
func (w *watch) Events() <-chan coord.Event {
	return w.events
}

// Close stops the watch and waits for its streams to end.
func (w *watch) Close() error {
	w.once.Do(func() {
		w.cancel()
		w.err = w.inner.Close()
		<-w.done
	})
	return w.err
}

// forward sends each inner event at the current commit sequence.
func (w *watch) forward(ctx context.Context, db *db_s4db.DB) {
	events := w.inner.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			event.Generation = db.Seq()
			if !w.send(ctx, event) {
				return
			}
		}
	}
}

// commits sends an event for each commit sequence after seq the database
// publishes, from this process or another, until ctx ends or db closes.
func (w *watch) commits(ctx context.Context, db *db_s4db.DB, scope coord.Scope, seq uint64) {
	for db.WaitSeq(ctx, seq+1) == nil {
		seq = db.Seq()
		event := coord.Event{
			VolumeID:      scope.VolumeID,
			ObjectStoreID: scope.ObjectStoreID,
			Generation:    seq,
		}
		if !w.send(ctx, event) {
			return
		}
	}
}

// send delivers event, reporting false once ctx ends.
func (w *watch) send(ctx context.Context, event coord.Event) bool {
	select {
	case w.events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// _ is a type assertion
var _ coord.Watch = (*watch)(nil)
