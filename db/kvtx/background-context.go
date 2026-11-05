package kvtx

import "context"

// backgroundWriteContextKey marks a context whose write transactions are
// background work.
type backgroundWriteContextKey struct{}

// WithBackgroundWriteContext marks the write transactions opened with ctx as
// background work, such as cache fills and bookkeeping a later pass can redo.
// A store with a prioritized writer lock grants it to them only when no
// foreground writer waits, so they never delay a foreground write by more
// than the one they hold. Under constant foreground load they may wait
// indefinitely. Stores without a prioritized writer ignore the mark.
func WithBackgroundWriteContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundWriteContextKey{}, true)
}

// IsBackgroundWriteContext reports whether ctx carries
// WithBackgroundWriteContext.
func IsBackgroundWriteContext(ctx context.Context) bool {
	background, _ := ctx.Value(backgroundWriteContextKey{}).(bool)
	return background
}
