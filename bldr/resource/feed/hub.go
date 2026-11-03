package feed

import (
	"context"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/world/viewstream"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world/keyedview"
)

// Hub holds the running feeds of one World.
type Hub[S any] struct {
	// options binds the application source without owning its policy.
	options Options[S]
	// linger is how long a feed runs after its last watcher leaves.
	linger time.Duration
	// history is how many Views each feed's History holds.
	history int

	// mtx guards runs, and is taken before a run's mtx.
	mtx sync.Mutex
	// runs holds each running feed by Feed and key.
	runs map[feedID]any
}

// feedID names one running feed.
type feedID struct {
	// feed is the *Feed[S, T].
	feed any
	// key is the feed's key.
	key string
}

// NewHub constructs a hub with no running feeds.
func NewHub[S any](linger time.Duration, history int, options Options[S]) *Hub[S] {
	return &Hub[S]{linger: linger, history: history, options: options, runs: make(map[feedID]any)}
}

// feedRun is one running watch of a feed's key and the streams that follow
// it.
type feedRun[S, T any] struct {
	// hub is the hub the run belongs to.
	hub *Hub[S]
	// id names the run in the hub.
	id feedID
	// cancel stops the watch.
	cancel context.CancelFunc
	// pokes reruns the watch at once after a watcher's own write.
	pokes *viewstream.Pokes
	// history remembers the recent Views.
	history *keyedview.History

	// mtx guards the fields below.
	mtx sync.Mutex
	// watchers counts the streams following the run.
	watchers int
	// stop is the timer that ends the run after the last watcher left, or
	// nil while one follows it.
	stop *time.Timer
	// lingers counts the timers set, so a timer that fires after a watcher
	// stopped it does not end the run.
	lingers uint64
	// pub is the latest publication.
	pub *Publication[T]
}

// joinFeed adds a watcher to the running watch of feed's key, starting one
// over s, at the priority ctx carries, when none runs. The returned function
// removes the watcher; the watch ends a linger after the last one leaves.
func joinFeed[S, T any](ctx context.Context, hub *Hub[S], s S, feed *Feed[S, T], key string) (*feedRun[S, T], func()) {
	// Find or start the one source for this descriptor and key.
	id := feedID{feed: feed, key: key}
	hub.mtx.Lock()
	defer hub.mtx.Unlock()
	run, _ := hub.runs[id].(*feedRun[S, T])
	if run == nil {
		// The watch outlives the stream that starts it, but builds its first
		// view at that stream's priority.
		watchCtx, cancel := context.WithCancel(hub.options.Context(ctx))
		run = &feedRun[S, T]{
			hub:     hub,
			id:      id,
			cancel:  cancel,
			pokes:   viewstream.NewPokes(),
			history: keyedview.NewHistory(hub.history),
			pub:     &Publication[T]{next: make(chan struct{})},
		}
		hub.runs[id] = run
		store := hub.options.Public(s)
		if feed.Bound {
			store = s
		}
		go func() {
			defer cancel()
			run.watch(watchCtx, store, feed, key)
		}()
	}

	// Cancel pending linger while this subscriber owns the source.
	run.mtx.Lock()
	run.watchers++
	if run.stop != nil {
		run.stop.Stop()
		run.stop = nil
	}
	run.mtx.Unlock()
	return run, run.leave
}

// leave removes a watcher, ending the run a linger after the last one.
func (r *feedRun[S, T]) leave() {
	// Serialize subscriber and timer changes with hub membership.
	r.hub.mtx.Lock()
	defer r.hub.mtx.Unlock()
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.watchers--; r.watchers != 0 {
		return
	}
	r.lingers++
	linger := r.lingers
	r.stop = time.AfterFunc(r.hub.linger, func() { r.expire(linger) })
}

// expire ends the run when the timer of linger is still pending: no watcher
// joined since it was set.
func (r *feedRun[S, T]) expire(linger uint64) {
	// Serialize subscriber and timer changes with hub membership.
	r.hub.mtx.Lock()
	defer r.hub.mtx.Unlock()
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.stop == nil || r.lingers != linger {
		return
	}
	r.stop = nil
	r.cancel()
	r.remove()
}

// remove takes the run out of the hub, so the next watcher starts another.
// The caller holds the hub's mtx.
func (r *feedRun[S, T]) remove() {
	if run, ok := r.hub.runs[r.id]; ok && run == any(r) {
		delete(r.hub.runs, r.id)
	}
}

// watch publishes key's view of each revision that changes it, paced by the
// view interval, until ctx ends or the view cannot be read. It ends the
// watchers with the error then; a watcher that joins next starts a new run.
func (r *feedRun[S, T]) watch(ctx context.Context, store S, feed *Feed[S, T], key string) {
	// Publish each changed read under the source's watch lifetime.
	var last T
	published := false
	var builder keyedview.Builder
	err := r.hub.options.Watch(ctx, store, r.pokes, func(ctx context.Context) error {
		// Read the view at a revision no later than the one it reflects.
		revision, err := r.hub.options.Revision(ctx, store)
		if err != nil {
			return err
		}
		value, err := feed.Read(ctx, store, key)
		if err != nil {
			return err
		}
		if published && feed.Equal != nil && feed.Equal(last, value) {
			return nil
		}

		// Name its records and publish it.
		pub := &Publication[T]{Value: value, Revision: revision, published: true, encode: feed.Encode, history: r.history}
		switch {
		case feed.Records != nil:
			scope, records := feed.Records(value)
			pub.View, err = builder.Build(scope, records)
		case feed.Encode != nil:
			data, encodeErr := pub.Encoded()
			if encodeErr != nil {
				return encodeErr
			}
			pub.View, err = keyedview.Build(key, map[string]rawRecord{"": data})
		}
		if err != nil {
			return err
		}
		if pub.View != nil {
			r.history.Add(pub.View)
		}
		last, published = value, true
		r.publish(pub)
		return nil
	})

	// A run the hub stopped ends quietly; one whose view failed ends its
	// watchers with the error.
	stopped := ctx.Err() != nil
	r.cancel()
	if stopped {
		return
	}
	if err == nil {
		err = errors.New("feed watch ended")
	}
	r.hub.mtx.Lock()
	r.remove()
	r.hub.mtx.Unlock()
	r.publish(&Publication[T]{err: err})
}

// publish replaces the latest publication with pub and wakes the watchers
// waiting for one.
func (r *feedRun[S, T]) publish(pub *Publication[T]) {
	// Replace the head under the lock, then wake readers of the old head.
	pub.next = make(chan struct{})
	r.mtx.Lock()
	old := r.pub
	r.pub = pub
	r.mtx.Unlock()
	close(old.next)
}

// latest returns the latest publication.
func (r *feedRun[S, T]) latest() *Publication[T] {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.pub
}

// WatchFeed calls deliver with the latest publication of feed's key, once the
// feed has one, and again with each newer one, until ctx ends, deliver fails
// or the feed cannot read its view. Every stream of a World watching one key
// shares one watch and its publications, which deliver must not modify; a
// stream behind receives the latest publication, skipping the ones between.
// A source bound to an actor has the feed rerun at once after the actor's own
// write, instead of waiting out the interval. Its access is checked once as
// the stream joins a feed that acts for no actor, whose view is public;
// session renewal ends the connection of an actor whose access ends later.
func WatchFeed[S, T any](ctx context.Context, hub *Hub[S], s S, feed *Feed[S, T], key string, deliver func(*Publication[T]) error) error {
	// Join through the same admission boundary as manually combined subscriptions.
	sub, err := Subscribe(ctx, hub, s, feed, key)
	if err != nil {
		return err
	}
	defer sub.Close()
	run := sub.run

	// A bound feed's own watch passes its actor's writes; an unbound one
	// needs a poke from the watcher whose actor wrote.
	var ownSince func(uint64) (bool, <-chan struct{})
	if !feed.Bound {
		ownSince = hub.options.Own(s)
	}
	var seen *Publication[T]
	var poked uint64
	for {
		// Deliver the latest publication once.
		pub := run.latest()
		if pub.err != nil {
			return pub.err
		}
		if pub.published && pub != seen {
			seen = pub
			if err := deliver(pub); err != nil {
				return err
			}
		}

		// Rerun the feed at once after the actor's write past what they
		// hold.
		var own <-chan struct{}
		if ownSince != nil && seen != nil {
			var wrote bool
			if wrote, own = ownSince(max(seen.Revision, poked)); wrote {
				head, err := hub.options.Revision(ctx, s)
				if err != nil {
					return err
				}
				poked = head
				run.pokes.Poke()
				continue
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pub.next:
		case <-own:
		}
	}
}
