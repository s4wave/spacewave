package feed

import (
	"context"
	"sync"
)

// Subscription holds one reference to the shared source. Its caller may combine
// publications with application events, such as a revision-specific projection.
// Close releases the source once; Snapshot's returned view is immutable.
type Subscription[S, T any] struct {
	// run owns the publications this subscriber follows.
	run *feedRun[S, T]
	// once prevents releasing the subscriber twice.
	once sync.Once
}

// Subscribe checks public admission and joins the shared source for feed/key.
// Bound feeds require the caller to include its authority scope in the key.
func Subscribe[S, T any](ctx context.Context, hub *Hub[S], source S, feed *Feed[S, T], key string) (*Subscription[S, T], error) {
	// Admit the subscriber before it can observe a public publication.
	if !feed.Bound {
		if err := hub.options.Admit(ctx, source); err != nil {
			return nil, err
		}
	}

	// Hold the source until this subscriber closes its reference.
	run, _ := joinFeed(ctx, hub, source, feed, key)
	return &Subscription[S, T]{run: run}, nil
}

// Snapshot returns the current publication and a channel closed by its
// replacement. A nil publication precedes the first read; an error ends this
// source and a later subscription can start a new one. Sample before reading
// application events, then wait on the sampled channel to avoid missed changes.
func (s *Subscription[S, T]) Snapshot() (*Publication[T], <-chan struct{}, error) {
	// Sample the publication and its wake channel together.
	pub := s.run.latest()
	if pub.err != nil {
		return nil, pub.next, pub.err
	}
	if !pub.published {
		return nil, pub.next, nil
	}
	return pub, pub.next, nil
}

// Close releases the subscriber and starts linger after the last one leaves.
func (s *Subscription[S, T]) Close() { s.once.Do(s.run.leave) }
