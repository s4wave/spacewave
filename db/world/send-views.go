package world

import "context"

// View is a generated message whose content SendViews can compare.
type View[T any] interface {
	// EqualVT reports whether two messages carry the same content.
	EqualVT(T) bool
}

// SendViews sends the first projection and each later unequal projection. It is
// SendViewsWith with zero options.
func SendViews[T View[T]](
	ctx context.Context,
	source WorldWaitSeqno,
	read func(context.Context, WorldState) (T, error),
	send func(T) error,
) error {
	return SendViewsWith(ctx, source, WatchViewOptions{}, read, send)
}

// SendViewsWith sends the first projection and each later unequal projection,
// evaluating like WatchViewWith with opts. source and read follow
// WatchViewWith's contract, including context-carried transactions and
// RerunAt. read must return an immutable message. send must not mutate it.
// Returns when ctx ends or read or send fails.
func SendViewsWith[T View[T]](
	ctx context.Context,
	source WorldWaitSeqno,
	opts WatchViewOptions,
	read func(context.Context, WorldState) (T, error),
	send func(T) error,
) error {
	// Retain the last successfully sent projection for this subscription.
	var previous T
	sent := false

	// Evaluate on relevant changes, suppressing equal projections.
	return WatchViewWith(ctx, source, opts, func(ctx context.Context, ws WorldState) error {
		// Compute the next immutable projection.
		next, err := read(ctx, ws)
		if err != nil {
			return err
		}
		if sent && next.EqualVT(previous) {
			return nil
		}

		// Advance the comparison baseline only after sending succeeds.
		if err := send(next); err != nil {
			return err
		}
		previous, sent = next, true
		return nil
	})
}
