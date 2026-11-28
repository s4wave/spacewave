package space_world

import "context"

// queuingKey is the context key of the queue place of the admission whose root
// moves the turn queues for its durable step.
type queuingKey struct{}

// withQueuing returns ctx carrying an admission's queue place, so the root
// moves made under it queue for that admission's durable step instead of
// landing in the turn.
func withQueuing(ctx context.Context, owner *admissionMoves) context.Context {
	return context.WithValue(ctx, queuingKey{}, owner)
}

// queuing returns the queue place withQueuing set on ctx, or nil.
func queuing(ctx context.Context) *admissionMoves {
	owner, _ := ctx.Value(queuingKey{}).(*admissionMoves)
	return owner
}
