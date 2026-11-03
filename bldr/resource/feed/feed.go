package feed

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/db/world/keyedview"
)

// Message is a view sent as read: compared with the last one, so an equal
// view is not published, and encoded once for every watcher.
type Message[T any] interface {
	world.View[T]
	// MarshalVT encodes the view; equal views encode equally.
	MarshalVT() ([]byte, error)
}

// Feed is one kind of view that many streams watch, such as the health of the
// service or one actor's devices. Each key of a Feed has at most one running
// watch per World, which reads and encodes each change once for every stream
// watching that key. A Feed is declared once, as a package variable, and
// keeps no state of its own.
type Feed[S, T any] struct {
	// Read reads the view of key. Its result is shared by every watcher, so
	// it must not be modified after Read returns.
	Read func(ctx context.Context, store S, key string) (T, error)
	// Bound runs the feed over the source of the stream that starts it, for
	// views that act for one account: key must then name that account, so
	// every stream of a key acts for the same one. Otherwise the feed acts
	// for no actor, and its view must hold only what every watcher may see.
	Bound bool
	// Equal reports whether two views are equal, so the later one is not
	// published, or is nil to publish every read.
	Equal func(earlier, later T) bool
	// Encode encodes a view that streams send as read, naming its version
	// when Records is nil. Nil for views each stream projects before
	// sending, which then carry no encoding and, without Records, no
	// version.
	Encode func(T) ([]byte, error)
	// Records splits a view into its keyed records, so a watcher learns
	// which records changed, or is nil to treat the encoded view as one
	// record.
	Records func(T) (scope string, records map[string]keyedview.Record)
}

// MessageFeed returns feed for a view sent as read, compared with EqualVT and
// encoded with MarshalVT.
func MessageFeed[S any, T Message[T]](feed Feed[S, T]) *Feed[S, T] {
	feed.Equal = func(earlier, later T) bool { return later.EqualVT(earlier) }
	feed.Encode = func(view T) ([]byte, error) { return view.MarshalVT() }
	return &feed
}
