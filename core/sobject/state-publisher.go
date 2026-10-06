package sobject

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// StatePublisher is implemented by a SharedObject whose state shares the
// atomic publication domain of its block store. When the context passed to
// QueueOperation carries a PublishStateFunc, the provider calls it exactly
// once before the operation's state is committed.
type StatePublisher interface {
	// LocalStateHead returns a head update that sets key to value in the local
	// state store storeID, for a publication through the block store.
	LocalStateHead(storeID string, key, value []byte) *block.AtomicHeadUpdate
}

// PublishStateFunc writes the World commit that queued an operation. Given
// the head of the operation's state write, it publishes the head atomically
// with the commit and reports true; the state is then committed. Given nil,
// or when the block store cannot publish, it writes the commit's blocks and
// reports false; the provider then commits the state itself.
type PublishStateFunc func(ctx context.Context, head *block.AtomicHeadUpdate) (bool, error)

// publishStateKey is the context key of a PublishStateFunc.
type publishStateKey struct{}

// WithPublishState returns ctx carrying fn for the operation queued with it.
func WithPublishState(ctx context.Context, fn PublishStateFunc) context.Context {
	return context.WithValue(ctx, publishStateKey{}, fn)
}

// GetPublishState returns the PublishStateFunc ctx carries, or nil.
func GetPublishState(ctx context.Context) PublishStateFunc {
	fn, _ := ctx.Value(publishStateKey{}).(PublishStateFunc)
	return fn
}
